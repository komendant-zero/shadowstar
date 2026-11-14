package dns

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net"
	"strconv"
	"strings"
)

const (
	TypeA     uint16 = 1
	TypeNS    uint16 = 2
	TypeCNAME uint16 = 5
	TypePTR   uint16 = 12
	TypeAAAA  uint16 = 28
	ClassINET uint16 = 1

	RCodeNoError   = 0
	RCodeFormatErr = 1
	RCodeServFail  = 2
	RCodeNXDomain  = 3
)

// Question represents a DNS query question entry.
type Question struct {
	Name  string
	Type  uint16
	Class uint16
}

// ResourceRecord represents a DNS answer or authority resource record.
type ResourceRecord struct {
	Name  string
	Type  uint16
	Class uint16
	TTL   uint32
	Data  []byte
}

// Message represents an RFC 1035 wireformat DNS message.
type Message struct {
	ID                 uint16
	Response           bool
	Opcode             int
	Authoritative      bool
	Truncated          bool
	RecursionDesired   bool
	RecursionAvailable bool
	RCode              int
	Questions          []Question
	Answers            []ResourceRecord
}

// ParseMessage decodes binary RFC 1035 wireformat bytes into a Message struct.
func ParseMessage(data []byte) (*Message, error) {
	if len(data) < 12 {
		return nil, errors.New("dns message too short (<12 bytes)")
	}

	msg := &Message{
		ID: binary.BigEndian.Uint16(data[0:2]),
	}
	flags := binary.BigEndian.Uint16(data[2:4])
	msg.Response = (flags & 0x8000) != 0
	msg.Opcode = int((flags >> 11) & 0x0F)
	msg.Authoritative = (flags & 0x0400) != 0
	msg.Truncated = (flags & 0x0200) != 0
	msg.RecursionDesired = (flags & 0x0100) != 0
	msg.RecursionAvailable = (flags & 0x0080) != 0
	msg.RCode = int(flags & 0x000F)

	qdCount := int(binary.BigEndian.Uint16(data[4:6]))
	anCount := int(binary.BigEndian.Uint16(data[6:8]))

	offset := 12
	for i := 0; i < qdCount; i++ {
		name, nextOff, err := decodeName(data, offset)
		if err != nil {
			return nil, err
		}
		if nextOff+4 > len(data) {
			return nil, errors.New("truncated question section")
		}
		qType := binary.BigEndian.Uint16(data[nextOff : nextOff+2])
		qClass := binary.BigEndian.Uint16(data[nextOff+2 : nextOff+4])
		offset = nextOff + 4
		msg.Questions = append(msg.Questions, Question{Name: name, Type: qType, Class: qClass})
	}

	for i := 0; i < anCount; i++ {
		name, nextOff, err := decodeName(data, offset)
		if err != nil {
			return nil, err
		}
		if nextOff+10 > len(data) {
			return nil, errors.New("truncated answer header")
		}
		aType := binary.BigEndian.Uint16(data[nextOff : nextOff+2])
		aClass := binary.BigEndian.Uint16(data[nextOff+2 : nextOff+4])
		ttl := binary.BigEndian.Uint32(data[nextOff+4 : nextOff+8])
		rdLen := int(binary.BigEndian.Uint16(data[nextOff+8 : nextOff+10]))
		nextOff += 10
		if nextOff+rdLen > len(data) {
			return nil, errors.New("truncated answer rdata")
		}
		rData := make([]byte, rdLen)
		copy(rData, data[nextOff:nextOff+rdLen])
		offset = nextOff + rdLen
		msg.Answers = append(msg.Answers, ResourceRecord{
			Name:  name,
			Type:  aType,
			Class: aClass,
			TTL:   ttl,
			Data:  rData,
		})
	}

	return msg, nil
}

func encodeName(name string) []byte {
	var buf bytes.Buffer
	parts := strings.Split(strings.TrimSuffix(name, "."), ".")
	for _, p := range parts {
		if len(p) == 0 {
			continue
		}
		buf.WriteByte(byte(len(p)))
		buf.WriteString(p)
	}
	buf.WriteByte(0x00)
	return buf.Bytes()
}

func decodeName(data []byte, offset int) (string, int, error) {
	var labels []string
	visited := make(map[int]bool)
	origOffset := offset
	jumped := false
	hops := 0

	for {
		if offset >= len(data) {
			return "", 0, errors.New("offset out of bounds decoding name")
		}
		length := int(data[offset])
		if length == 0 {
			if !jumped {
				origOffset = offset + 1
			}
			break
		}
		if (length & 0xC0) == 0xC0 {
			if offset+1 >= len(data) {
				return "", 0, errors.New("incomplete compression pointer")
			}
			ptr := int(binary.BigEndian.Uint16(data[offset:offset+2]) & 0x3FFF)
			if visited[ptr] || hops > 10 {
				return "", 0, errors.New("compression pointer loop detected")
			}
			visited[ptr] = true
			hops++
			if !jumped {
				origOffset = offset + 2
				jumped = true
			}
			offset = ptr
			continue
		}
		offset++
		if offset+length > len(data) {
			return "", 0, errors.New("label extends beyond packet")
		}
		labels = append(labels, string(data[offset:offset+length]))
		offset += length
	}
	return strings.Join(labels, "."), origOffset, nil
}

// BuildQuery constructs an RFC 1035 wireformat DNS query packet.
func BuildQuery(domain string, qType uint16, id uint16) []byte {
	buf := make([]byte, 12)
	binary.BigEndian.PutUint16(buf[0:2], id)
	binary.BigEndian.PutUint16(buf[2:4], 0x0100) // RD=1
	binary.BigEndian.PutUint16(buf[4:6], 1)      // QDCOUNT=1
	qname := encodeName(domain)
	buf = append(buf, qname...)
	tail := make([]byte, 4)
	binary.BigEndian.PutUint16(tail[0:2], qType)
	binary.BigEndian.PutUint16(tail[2:4], ClassINET)
	return append(buf, tail...)
}

// BuildResponse constructs an RFC 1035 wireformat DNS response packet with compression pointer answers.
func BuildResponse(req *Message, answers []ResourceRecord, rcode int) []byte {
	buf := make([]byte, 12)
	binary.BigEndian.PutUint16(buf[0:2], req.ID)
	flags := uint16(0x8180) | uint16(rcode&0x0F)
	if req.RecursionDesired {
		flags |= 0x0100
	}
	binary.BigEndian.PutUint16(buf[2:4], flags)
	binary.BigEndian.PutUint16(buf[4:6], uint16(len(req.Questions)))
	binary.BigEndian.PutUint16(buf[6:8], uint16(len(answers)))

	for _, q := range req.Questions {
		buf = append(buf, encodeName(q.Name)...)
		tail := make([]byte, 4)
		binary.BigEndian.PutUint16(tail[0:2], q.Type)
		binary.BigEndian.PutUint16(tail[2:4], q.Class)
		buf = append(buf, tail...)
	}

	for _, ans := range answers {
		buf = append(buf, 0xC0, 0x0C) // Compression pointer to question name at offset 12
		tail := make([]byte, 10)
		binary.BigEndian.PutUint16(tail[0:2], ans.Type)
		binary.BigEndian.PutUint16(tail[2:4], ans.Class)
		binary.BigEndian.PutUint32(tail[4:8], ans.TTL)
		binary.BigEndian.PutUint16(tail[8:10], uint16(len(ans.Data)))
		buf = append(buf, tail...)
		buf = append(buf, ans.Data...)
	}
	return buf
}

// ParsePTR extracts an IPv4 address from a reverse lookup domain (e.g. "2.0.18.198.in-addr.arpa").
func ParsePTR(name string) net.IP {
	name = strings.TrimSuffix(strings.ToLower(name), ".in-addr.arpa")
	parts := strings.Split(name, ".")
	if len(parts) != 4 {
		return nil
	}
	b0, err0 := strconv.Atoi(parts[3])
	b1, err1 := strconv.Atoi(parts[2])
	b2, err2 := strconv.Atoi(parts[1])
	b3, err3 := strconv.Atoi(parts[0])
	if err0 != nil || err1 != nil || err2 != nil || err3 != nil {
		return nil
	}
	if b0 < 0 || b0 > 255 || b1 < 0 || b1 > 255 || b2 < 0 || b2 > 255 || b3 < 0 || b3 > 255 {
		return nil
	}
	return net.IPv4(byte(b0), byte(b1), byte(b2), byte(b3))
}
