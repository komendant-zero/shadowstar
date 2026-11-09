package protocol

import (
	"bytes"
	"crypto/ecdh"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

const (
	TLSRecordHandshake     = 0x16
	TLSVersion10           = 0x0301
	TLSVersion12           = 0x0303
	TLSVersion13           = 0x0304
	TLSHandshakeClientHello = 0x01
	TLSHandshakeServerHello = 0x02

	ExtServerName         = 0x0000
	ExtSupportedGroups    = 0x000a
	ExtSignatureAlgorithms = 0x000d
	ExtALPN               = 0x0010
	ExtKeyShare           = 0x0033
	ExtSupportedVersions  = 0x002b
	ExtPadding            = 0x0015

	GroupX25519 = 0x001d

	AuthTagLen = 16
	AuthWindow = 900 // seconds validity window for handshake replay defense (15 minutes)
)

// HandshakeAuth holds authentication payload embedded in TLS SessionID
type HandshakeAuth struct {
	Timestamp uint64 // 8 bytes
	Salt      uint64 // 8 bytes
	Tag       [16]byte // 16 bytes HMAC
}

// BuildAuthSessionID creates a 32-byte TLS SessionID containing replay-protected authentication
func BuildAuthSessionID(masterKey []byte, clientPub []byte, sni string, timeOffsetSec int64) ([]byte, error) {
	ts := uint64(time.Now().Unix() + timeOffsetSec)
	saltBytes := RandomBytes(8)
	salt := binary.BigEndian.Uint64(saltBytes)

	data := make([]byte, 16+len(clientPub)+len(sni))
	binary.BigEndian.PutUint64(data[0:8], ts)
	binary.BigEndian.PutUint64(data[8:16], salt)
	copy(data[16:16+len(clientPub)], clientPub)
	copy(data[16+len(clientPub):], []byte(sni))

	tag := ComputeAuthTag(masterKey, data)

	sessionID := make([]byte, 32)
	binary.BigEndian.PutUint64(sessionID[0:8], ts)
	binary.BigEndian.PutUint64(sessionID[8:16], salt)
	copy(sessionID[16:32], tag[:16])

	return sessionID, nil
}

// VerifyAuthSessionID validates the SessionID against masterKey, clientPub, and SNI
func VerifyAuthSessionID(sessionID, masterKey, clientPub []byte, sni string) bool {
	if len(sessionID) != 32 {
		return false
	}

	ts := binary.BigEndian.Uint64(sessionID[0:8])
	salt := binary.BigEndian.Uint64(sessionID[8:16])
	providedTag := sessionID[16:32]

	now := uint64(time.Now().Unix())
	if ts > now+AuthWindow || ts < now-AuthWindow {
		return false
	}

	data := make([]byte, 16+len(clientPub)+len(sni))
	binary.BigEndian.PutUint64(data[0:8], ts)
	binary.BigEndian.PutUint64(data[8:16], salt)
	copy(data[16:16+len(clientPub)], clientPub)
	copy(data[16+len(clientPub):], []byte(sni))

	expectedTag := ComputeAuthTag(masterKey, data)
	return VerifyAuthTag(masterKey, data, providedTag) && len(expectedTag) >= 16
}

// BuildClientHello constructs a fully standard-compliant TLS 1.3 ClientHello packet
func BuildClientHello(sni string, masterKey []byte, clientPub []byte, paddingLen int, timeOffsetSec int64) ([]byte, error) {
	sessionID, err := BuildAuthSessionID(masterKey, clientPub, sni, timeOffsetSec)
	if err != nil {
		return nil, err
	}

	var extensions bytes.Buffer

	// 1. SNI Extension
	sniBytes := []byte(sni)
	sniData := make([]byte, len(sniBytes)+5)
	binary.BigEndian.PutUint16(sniData[0:2], uint16(len(sniBytes)+3))
	sniData[2] = 0x00 // host_name type
	binary.BigEndian.PutUint16(sniData[3:5], uint16(len(sniBytes)))
	copy(sniData[5:], sniBytes)
	writeExtension(&extensions, ExtServerName, sniData)

	// 2. Supported Versions (TLS 1.3, TLS 1.2)
	writeExtension(&extensions, ExtSupportedVersions, []byte{0x04, 0x03, 0x04, 0x03, 0x03})

	// 3. Supported Groups (X25519, secp256r1)
	writeExtension(&extensions, ExtSupportedGroups, []byte{0x00, 0x04, 0x00, 0x1d, 0x00, 0x17})

	// 4. Signature Algorithms
	sigAlgs := []byte{
		0x00, 0x08,
		0x04, 0x03, // ecdsa_secp256r1_sha256
		0x08, 0x04, // rsa_pss_rsae_sha256
		0x04, 0x01, // rsa_pkcs1_sha256
		0x05, 0x01, // rsa_pkcs1_sha384
	}
	writeExtension(&extensions, ExtSignatureAlgorithms, sigAlgs)

	// 5. ALPN (h2, http/1.1)
	alpnData := []byte{
		0x00, 0x0c,
		0x02, 'h', '2',
		0x08, 'h', 't', 't', 'p', '/', '1', '.', '1',
	}
	writeExtension(&extensions, ExtALPN, alpnData)

	// 6. Key Share (X25519)
	keyShareData := make([]byte, 2+2+2+len(clientPub))
	binary.BigEndian.PutUint16(keyShareData[0:2], uint16(4+len(clientPub)))
	binary.BigEndian.PutUint16(keyShareData[2:4], GroupX25519)
	binary.BigEndian.PutUint16(keyShareData[4:6], uint16(len(clientPub)))
	copy(keyShareData[6:], clientPub)
	writeExtension(&extensions, ExtKeyShare, keyShareData)

	// 7. Dynamic Padding Extension (to randomize packet sizes & defeat length profiling)
	if paddingLen < 16 {
		paddingLen = 16
	}
	if paddingLen > 256 {
		paddingLen = 256
	}
	padBytes := make([]byte, paddingLen)
	writeExtension(&extensions, ExtPadding, padBytes)

	// Assemble ClientHello body
	var body bytes.Buffer
	// Client Version 0x0303 (TLS 1.2)
	body.Write([]byte{0x03, 0x03})
	// Random 32 bytes
	body.Write(RandomBytes(32))
	// Session ID length and data
	body.WriteByte(byte(len(sessionID)))
	body.Write(sessionID)

	// Cipher Suites (standard Chrome/Firefox TLS 1.3 list)
	ciphers := []byte{
		0x00, 0x0a,
		0x13, 0x01, // TLS_AES_128_GCM_SHA256
		0x13, 0x02, // TLS_AES_256_GCM_SHA384
		0x13, 0x03, // TLS_CHACHA20_POLY1305_SHA256
		0xc0, 0x2b, // ECDHE-ECDSA-AES128-GCM-SHA256
		0xc0, 0x2f, // ECDHE-RSA-AES128-GCM-SHA256
	}
	body.Write(ciphers)

	// Compression methods: 1 byte len, 0x00 (null)
	body.Write([]byte{0x01, 0x00})

	// Extensions length and data
	extBytes := extensions.Bytes()
	binary.Write(&body, binary.BigEndian, uint16(len(extBytes)))
	body.Write(extBytes)

	// Assemble TLS Record Layer
	clientHelloBytes := body.Bytes()
	var record bytes.Buffer
	record.WriteByte(TLSRecordHandshake)
	record.Write([]byte{0x03, 0x01}) // Record version TLS 1.0
	binary.Write(&record, binary.BigEndian, uint16(len(clientHelloBytes)+4))

	// Handshake header
	record.WriteByte(TLSHandshakeClientHello)
	record.WriteByte(0x00) // 24-bit length
	binary.Write(&record, binary.BigEndian, uint16(len(clientHelloBytes)))
	record.Write(clientHelloBytes)

	return record.Bytes(), nil
}

// BuildServerHello constructs a standard TLS 1.3 ServerHello packet carrying server's X25519 public key
func BuildServerHello(serverPub []byte, sessionID []byte) []byte {
	var body bytes.Buffer
	// Server Version 0x0303
	body.Write([]byte{0x03, 0x03})
	// Server Random (32 bytes)
	body.Write(RandomBytes(32))
	// Echo Session ID
	body.WriteByte(byte(len(sessionID)))
	body.Write(sessionID)
	// Selected Cipher: TLS_AES_256_GCM_SHA384
	body.Write([]byte{0x13, 0x02})
	// Compression: null
	body.WriteByte(0x00)

	// Extensions: Supported Versions (TLS 1.3) + Key Share
	var ext bytes.Buffer
	writeExtension(&ext, ExtSupportedVersions, []byte{0x03, 0x04})

	keyShareData := make([]byte, 4+len(serverPub))
	binary.BigEndian.PutUint16(keyShareData[0:2], GroupX25519)
	binary.BigEndian.PutUint16(keyShareData[2:4], uint16(len(serverPub)))
	copy(keyShareData[4:], serverPub)
	writeExtension(&ext, ExtKeyShare, keyShareData)

	extBytes := ext.Bytes()
	binary.Write(&body, binary.BigEndian, uint16(len(extBytes)))
	body.Write(extBytes)

	serverHelloBytes := body.Bytes()
	var record bytes.Buffer
	record.WriteByte(TLSRecordHandshake)
	record.Write([]byte{0x03, 0x03}) // Record version TLS 1.2
	binary.Write(&record, binary.BigEndian, uint16(len(serverHelloBytes)+4))

	record.WriteByte(TLSHandshakeServerHello)
	record.WriteByte(0x00)
	binary.Write(&record, binary.BigEndian, uint16(len(serverHelloBytes)))
	record.Write(serverHelloBytes)

	return record.Bytes()
}

func writeExtension(buf *bytes.Buffer, extType uint16, data []byte) {
	binary.Write(buf, binary.BigEndian, extType)
	binary.Write(buf, binary.BigEndian, uint16(len(data)))
	buf.Write(data)
}

// ParsedClientHello holds extracted handshake data
type ParsedClientHello struct {
	SessionID []byte
	SNI       string
	ClientPub []byte
	RawData   []byte
}

// ParseClientHello parses a TLS 1.3 ClientHello packet and extracts SessionID, SNI, and KeyShare
func ParseClientHello(raw []byte) (*ParsedClientHello, error) {
	if len(raw) < 5 {
		return nil, errors.New("packet too short for TLS record")
	}
	if raw[0] != TLSRecordHandshake {
		return nil, fmt.Errorf("not a TLS handshake record (type 0x%02x)", raw[0])
	}

	recordLen := binary.BigEndian.Uint16(raw[3:5])
	if len(raw) < int(5+recordLen) {
		return nil, errors.New("incomplete TLS record")
	}

	payload := raw[5 : 5+recordLen]
	if len(payload) < 4 || payload[0] != TLSHandshakeClientHello {
		return nil, errors.New("not a ClientHello handshake")
	}

	handshakeLen := int(payload[1])<<16 | int(payload[2])<<8 | int(payload[3])
	if len(payload) < 4+handshakeLen {
		return nil, errors.New("incomplete ClientHello body")
	}

	body := payload[4 : 4+handshakeLen]
	// Skip ClientVersion(2) + Random(32)
	offset := 34
	if len(body) < offset+1 {
		return nil, errors.New("truncated ClientHello")
	}

	sessionIDLen := int(body[offset])
	offset++
	if len(body) < offset+sessionIDLen {
		return nil, errors.New("truncated session ID")
	}
	sessionID := body[offset : offset+sessionIDLen]
	offset += sessionIDLen

	// Cipher suites
	if len(body) < offset+2 {
		return nil, errors.New("truncated cipher suites")
	}
	cipherLen := int(binary.BigEndian.Uint16(body[offset : offset+2]))
	offset += 2 + cipherLen

	// Compression methods
	if len(body) < offset+1 {
		return nil, errors.New("truncated compression")
	}
	compLen := int(body[offset])
	offset += 1 + compLen

	// Extensions
	if len(body) < offset+2 {
		return &ParsedClientHello{SessionID: sessionID, RawData: raw}, nil
	}
	extTotalLen := int(binary.BigEndian.Uint16(body[offset : offset+2]))
	offset += 2

	extData := body[offset:]
	if len(extData) > extTotalLen {
		extData = extData[:extTotalLen]
	}

	parsed := &ParsedClientHello{
		SessionID: sessionID,
		RawData:   raw,
	}

	// Parse extensions
	extOffset := 0
	for extOffset+4 <= len(extData) {
		extType := binary.BigEndian.Uint16(extData[extOffset : extOffset+2])
		extLen := int(binary.BigEndian.Uint16(extData[extOffset+2 : extOffset+4]))
		extOffset += 4

		if extOffset+extLen > len(extData) {
			break
		}
		data := extData[extOffset : extOffset+extLen]
		extOffset += extLen

		switch extType {
		case ExtServerName:
			if len(data) > 5 {
				nameLen := int(binary.BigEndian.Uint16(data[3:5]))
				if len(data) >= 5+nameLen {
					parsed.SNI = string(data[5 : 5+nameLen])
				}
			}
		case ExtKeyShare:
			// client_shares list length (2)
			if len(data) > 2 {
				shares := data[2:]
				shareOff := 0
				for shareOff+4 <= len(shares) {
					group := binary.BigEndian.Uint16(shares[shareOff : shareOff+2])
					keyLen := int(binary.BigEndian.Uint16(shares[shareOff+2 : shareOff+4]))
					shareOff += 4
					if shareOff+keyLen <= len(shares) && group == GroupX25519 && keyLen == 32 {
						parsed.ClientPub = shares[shareOff : shareOff+keyLen]
						break
					}
					shareOff += keyLen
				}
			}
		}
	}

	return parsed, nil
}

// ParseServerHello extracts Server's ephemeral public key from a ServerHello packet
func ParseServerHello(raw []byte) ([]byte, error) {
	if len(raw) < 5 || raw[0] != TLSRecordHandshake {
		return nil, errors.New("invalid TLS record for ServerHello")
	}
	recordLen := binary.BigEndian.Uint16(raw[3:5])
	if len(raw) < int(5+recordLen) {
		return nil, errors.New("incomplete ServerHello record")
	}

	payload := raw[5 : 5+recordLen]
	if len(payload) < 4 || payload[0] != TLSHandshakeServerHello {
		return nil, errors.New("not a ServerHello")
	}

	body := payload[4:]
	// Skip ServerVersion(2) + Random(32)
	offset := 34
	if len(body) < offset+1 {
		return nil, errors.New("truncated ServerHello")
	}
	sessionIDLen := int(body[offset])
	offset += 1 + sessionIDLen + 2 + 1 // sessionID + cipher(2) + compression(1)

	if len(body) < offset+2 {
		return nil, errors.New("missing ServerHello extensions")
	}
	extLen := int(binary.BigEndian.Uint16(body[offset : offset+2]))
	offset += 2

	extData := body[offset:]
	if len(extData) > extLen {
		extData = extData[:extLen]
	}

	extOffset := 0
	for extOffset+4 <= len(extData) {
		extType := binary.BigEndian.Uint16(extData[extOffset : extOffset+2])
		l := int(binary.BigEndian.Uint16(extData[extOffset+2 : extOffset+4]))
		extOffset += 4
		if extOffset+l > len(extData) {
			break
		}
		data := extData[extOffset : extOffset+l]
		extOffset += l

		if extType == ExtKeyShare && len(data) >= 4 {
			group := binary.BigEndian.Uint16(data[0:2])
			keyLen := int(binary.BigEndian.Uint16(data[2:4]))
			if group == GroupX25519 && len(data) >= 4+keyLen && keyLen == 32 {
				return data[4 : 4+keyLen], nil
			}
		}
	}
	return nil, errors.New("no X25519 key share found in ServerHello")
}

// PerformClientHandshake initiates the ShadowStar handshake over a net.Conn
func PerformClientHandshake(conn net.Conn, masterKey []byte, sni string, enableChunker bool, paddingLen int, timeOffsetSec int64) (*ecdh.PrivateKey, []byte, error) {
	clientPriv, clientPub, err := GenerateX25519KeyPair()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate client keypair: %w", err)
	}

	clientHello, err := BuildClientHello(sni, masterKey, clientPub, paddingLen, timeOffsetSec)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build ClientHello: %w", err)
	}

	// If TCP Chunker is enabled, fragment ClientHello to evade DPI SNI inspection
	if enableChunker && len(clientHello) > 32 {
		chunk1 := clientHello[:24]
		chunk2 := clientHello[24:]
		if _, err := conn.Write(chunk1); err != nil {
			return nil, nil, fmt.Errorf("chunk1 write failed: %w", err)
		}
		time.Sleep(2 * time.Millisecond) // micro-delay to ensure separate TCP segment
		if _, err := conn.Write(chunk2); err != nil {
			return nil, nil, fmt.Errorf("chunk2 write failed: %w", err)
		}
	} else {
		if _, err := conn.Write(clientHello); err != nil {
			return nil, nil, fmt.Errorf("client hello write failed: %w", err)
		}
	}

	// Read ServerHello
	buf := make([]byte, 4096)
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	n, err := io.ReadAtLeast(conn, buf, 5)
	if err != nil {
		return nil, nil, fmt.Errorf("server hello read failed: %w", err)
	}

	recordLen := int(binary.BigEndian.Uint16(buf[3:5]))
	for n < 5+recordLen {
		m, err := conn.Read(buf[n:])
		if err != nil {
			return nil, nil, fmt.Errorf("reading full ServerHello failed: %w", err)
		}
		n += m
	}
	_ = conn.SetReadDeadline(time.Time{})

	serverPub, err := ParseServerHello(buf[:n])
	if err != nil {
		return nil, nil, fmt.Errorf("invalid ServerHello: %w", err)
	}

	return clientPriv, serverPub, nil
}
