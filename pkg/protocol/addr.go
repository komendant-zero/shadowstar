package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
)

const (
	AtypIPv4   byte = 0x01
	AtypDomain byte = 0x03
	AtypIPv6   byte = 0x04
)

const (
	NetTCP byte = 0x01
	NetUDP byte = 0x02
)

// TargetAddress represents a parsed destination target (domain or IP + port)
type TargetAddress struct {
	Network byte
	Host    string
	Port    uint16
}

func (t TargetAddress) String() string {
	return net.JoinHostPort(t.Host, strconv.Itoa(int(t.Port)))
}

// ReadTargetAddress reads and parses a target address from a stream:
// [Network (1 byte)] [Atyp (1 byte)] [Host (varies)] [Port (2 bytes)]
func ReadTargetAddress(r io.Reader) (*TargetAddress, error) {
	netBuf := make([]byte, 1)
	if _, err := io.ReadFull(r, netBuf); err != nil {
		return nil, fmt.Errorf("failed to read network type: %w", err)
	}
	network := netBuf[0]
	if network != NetTCP && network != NetUDP {
		return nil, fmt.Errorf("unsupported network type: 0x%02x", network)
	}

	atypBuf := make([]byte, 1)
	if _, err := io.ReadFull(r, atypBuf); err != nil {
		return nil, fmt.Errorf("failed to read atyp: %w", err)
	}

	var host string
	switch atypBuf[0] {
	case AtypIPv4:
		ipBuf := make([]byte, 4)
		if _, err := io.ReadFull(r, ipBuf); err != nil {
			return nil, fmt.Errorf("failed to read ipv4: %w", err)
		}
		host = net.IP(ipBuf).String()

	case AtypDomain:
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(r, lenBuf); err != nil {
			return nil, fmt.Errorf("failed to read domain length: %w", err)
		}
		domainLen := int(lenBuf[0])
		domainBuf := make([]byte, domainLen)
		if _, err := io.ReadFull(r, domainBuf); err != nil {
			return nil, fmt.Errorf("failed to read domain name: %w", err)
		}
		host = string(domainBuf)

	case AtypIPv6:
		ipBuf := make([]byte, 16)
		if _, err := io.ReadFull(r, ipBuf); err != nil {
			return nil, fmt.Errorf("failed to read ipv6: %w", err)
		}
		host = net.IP(ipBuf).String()

	default:
		return nil, fmt.Errorf("unsupported atyp: 0x%02x", atypBuf[0])
	}

	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(r, portBuf); err != nil {
		return nil, fmt.Errorf("failed to read port: %w", err)
	}
	port := binary.BigEndian.Uint16(portBuf)

	return &TargetAddress{
		Network: network,
		Host:    host,
		Port:    port,
	}, nil
}

// WriteTargetAddress encodes and writes a destination address into the stream:
// [Network (1 byte)] [Atyp (1 byte)] [Host (varies)] [Port (2 bytes)]
func WriteTargetAddress(w io.Writer, network byte, addrStr string) error {
	if network == 0 {
		network = NetTCP
	}
	host, portStr, err := net.SplitHostPort(addrStr)
	if err != nil {
		return err
	}

	portNum, err := strconv.Atoi(portStr)
	if err != nil || portNum < 1 || portNum > 65535 {
		return errors.New("invalid port")
	}
	port := uint16(portNum)

	ip := net.ParseIP(host)
	if ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			// IPv4: [network(1), atyp(1), ip(4), port(2)]
			buf := make([]byte, 1+1+4+2)
			buf[0] = network
			buf[1] = AtypIPv4
			copy(buf[2:6], ip4)
			binary.BigEndian.PutUint16(buf[6:8], port)
			_, err = w.Write(buf)
			return err
		}
		// IPv6: [network(1), atyp(1), ip(16), port(2)]
		buf := make([]byte, 1+1+16+2)
		buf[0] = network
		buf[1] = AtypIPv6
		copy(buf[2:18], ip.To16())
		binary.BigEndian.PutUint16(buf[18:20], port)
		_, err = w.Write(buf)
		return err
	}

	// Domain: [network(1), atyp(1), len(1), domain(N), port(2)]
	domainBytes := []byte(host)
	if len(domainBytes) > 255 {
		return errors.New("domain too long")
	}
	buf := make([]byte, 1+1+1+len(domainBytes)+2)
	buf[0] = network
	buf[1] = AtypDomain
	buf[2] = byte(len(domainBytes))
	copy(buf[3:3+len(domainBytes)], domainBytes)
	binary.BigEndian.PutUint16(buf[3+len(domainBytes):], port)
	_, err = w.Write(buf)
	return err
}
