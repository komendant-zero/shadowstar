package protocol

import (
	"bytes"
	"crypto/ecdh"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

const (
	TLSRecordHandshake      = 0x16
	TLSVersion10            = 0x0301
	TLSVersion12            = 0x0303
	TLSVersion13            = 0x0304
	TLSHandshakeClientHello = 0x01
	TLSHandshakeServerHello = 0x02

	ExtServerName           = 0x0000
	ExtStatusRequest        = 0x0005 // RFC 6066
	ExtSupportedGroups      = 0x000a
	ExtECPointFormats       = 0x000b // RFC 8422
	ExtSignatureAlgorithms  = 0x000d
	ExtALPN                 = 0x0010
	ExtSCT                  = 0x0012 // RFC 6962
	ExtPadding              = 0x0015
	ExtExtendedMasterSecret = 0x0017 // RFC 7627
	ExtCompressCertificate  = 0x001b // RFC 8879
	ExtSessionTicket        = 0x0023 // RFC 5077
	ExtSupportedVersions    = 0x002b
	ExtPSKKeyExchangeModes  = 0x002d // RFC 8446
	ExtKeyShare             = 0x0033
	ExtApplicationSettings  = 0x4469 // ALPS (17513)
	ExtRenegotiationInfo    = 0xff01 // RFC 5746 (65281)

	GroupSecp256r1      = 0x0017
	GroupSecp384r1      = 0x0018
	GroupX25519         = 0x001d
	GroupX25519Kyber768 = 0x6399 // X25519Kyber768Draft00
	GroupX25519MLKEM768 = 0x11ec // X25519MLKEM768

	Kyber768PubKeyLen      = 1184
	Kyber768HybridShareLen = 1216 // 1184 + 32

	AuthTagLen = 16
	AuthWindow = 900 // seconds validity window for handshake replay defense (15 minutes)
)

var greaseValues = [...]uint16{
	0x0a0a, 0x1a1a, 0x2a2a, 0x3a3a,
	0x4a4a, 0x5a5a, 0x6a6a, 0x7a7a,
	0x8a8a, 0x9a9a, 0xaaaa, 0xbaba,
	0xcaca, 0xdada, 0xeaea, 0xfafa,
}

// RandomGREASE returns a pseudorandom RFC 8701 GREASE value
func RandomGREASE() uint16 {
	b := RandomBytes(1)
	return greaseValues[int(b[0]&0x0f)]
}

// IsGREASE checks if a uint16 is an RFC 8701 GREASE value
func IsGREASE(val uint16) bool {
	return (val&0x0f0f) == 0x0a0a && (val>>8) == (val&0xff)
}

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

// ReplayDetector prevents replaying previous TLS handshakes
type ReplayDetector struct {
	mu      sync.Mutex
	seen    map[[16]byte]int64 // map[ts(8)+salt(8)]timestamp
	cleanup time.Time
}

var defaultReplayDetector = NewReplayDetector()

func NewReplayDetector() *ReplayDetector {
	return &ReplayDetector{
		seen: make(map[[16]byte]int64),
	}
}

// Reset clears seen history (useful for tests)
func (r *ReplayDetector) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen = make(map[[16]byte]int64)
}

// CheckAndAdd checks if (ts, salt) was previously observed within AuthWindow
func (r *ReplayDetector) CheckAndAdd(sessionID []byte) bool {
	if len(sessionID) != 32 {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().Unix()
	if time.Since(r.cleanup) > 2*time.Minute {
		cutoff := now - AuthWindow
		for k, ts := range r.seen {
			if ts < cutoff {
				delete(r.seen, k)
			}
		}
		r.cleanup = time.Now()
	}

	var key [16]byte
	copy(key[:], sessionID[0:16])
	if _, exists := r.seen[key]; exists {
		return false // Replay attack detected
	}
	r.seen[key] = now
	return true
}

// VerifyAuthSessionID validates the SessionID against masterKey, clientPub, and SNI
func VerifyAuthSessionID(sessionID, masterKey, clientPub []byte, sni string) bool {
	return VerifyAuthSessionIDInternal(sessionID, masterKey, clientPub, sni, true)
}

// VerifyAuthSessionIDInternal allows toggling replay check for tests
func VerifyAuthSessionIDInternal(sessionID, masterKey, clientPub []byte, sni string, checkReplay bool) bool {
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

	if !VerifyAuthTag(masterKey, data, providedTag) {
		return false
	}

	if checkReplay {
		return defaultReplayDetector.CheckAndAdd(sessionID)
	}
	return true
}

// BuildClientHello constructs a Chrome (120-133) JA3/JA4 fingerprint-compliant TLS 1.3 ClientHello packet
func BuildClientHello(sni string, masterKey []byte, clientPub []byte, paddingLen int, timeOffsetSec int64) ([]byte, error) {
	sessionID, err := BuildAuthSessionID(masterKey, clientPub, sni, timeOffsetSec)
	if err != nil {
		return nil, err
	}

	var extensions bytes.Buffer

	// 1. GREASE Extension 1 (RFC 8701)
	greaseExt1 := RandomGREASE()
	writeExtension(&extensions, greaseExt1, nil)

	// 2. SNI Extension (RFC 6066)
	sniBytes := []byte(sni)
	sniData := make([]byte, len(sniBytes)+5)
	binary.BigEndian.PutUint16(sniData[0:2], uint16(len(sniBytes)+3))
	sniData[2] = 0x00 // host_name type
	binary.BigEndian.PutUint16(sniData[3:5], uint16(len(sniBytes)))
	copy(sniData[5:], sniBytes)
	writeExtension(&extensions, ExtServerName, sniData)

	// 3. Extended Master Secret (RFC 7627)
	writeExtension(&extensions, ExtExtendedMasterSecret, nil)

	// 4. Renegotiation Info (RFC 5746)
	writeExtension(&extensions, ExtRenegotiationInfo, []byte{0x00})

	// 5. Supported Groups (RFC 8422 & RFC 8701: GREASE, X25519Kyber768, X25519, secp256r1, secp384r1)
	greaseGroup := RandomGREASE()
	groupsData := make([]byte, 2+2*5)
	binary.BigEndian.PutUint16(groupsData[0:2], uint16(2*5))
	binary.BigEndian.PutUint16(groupsData[2:4], greaseGroup)
	binary.BigEndian.PutUint16(groupsData[4:6], GroupX25519Kyber768)
	binary.BigEndian.PutUint16(groupsData[6:8], GroupX25519)
	binary.BigEndian.PutUint16(groupsData[8:10], GroupSecp256r1)
	binary.BigEndian.PutUint16(groupsData[10:12], GroupSecp384r1)
	writeExtension(&extensions, ExtSupportedGroups, groupsData)

	// 6. EC Point Formats (RFC 8422)
	writeExtension(&extensions, ExtECPointFormats, []byte{0x01, 0x00})

	// 7. Session Ticket (RFC 5077)
	writeExtension(&extensions, ExtSessionTicket, nil)

	// 8. ALPN (RFC 7301: h2, http/1.1)
	alpnData := []byte{
		0x00, 0x0c,
		0x02, 'h', '2',
		0x08, 'h', 't', 't', 'p', '/', '1', '.', '1',
	}
	writeExtension(&extensions, ExtALPN, alpnData)

	// 9. Status Request (OCSP stapling - RFC 6066)
	writeExtension(&extensions, ExtStatusRequest, []byte{0x01, 0x00, 0x00, 0x00, 0x00})

	// 10. Signature Algorithms (RFC 8446 Chrome schemes list)
	sigAlgs := []byte{
		0x00, 0x10,
		0x04, 0x03, // ecdsa_secp256r1_sha256
		0x08, 0x04, // rsa_pss_rsae_sha256
		0x04, 0x01, // rsa_pkcs1_sha256
		0x05, 0x03, // ecdsa_secp384r1_sha384
		0x08, 0x05, // rsa_pss_rsae_sha384
		0x05, 0x01, // rsa_pkcs1_sha384
		0x08, 0x06, // rsa_pss_rsae_sha512
		0x06, 0x01, // rsa_pkcs1_sha512
	}
	writeExtension(&extensions, ExtSignatureAlgorithms, sigAlgs)

	// 11. Signed Certificate Timestamps (SCT - RFC 6962)
	writeExtension(&extensions, ExtSCT, nil)

	// 12. Key Share (GREASE share + X25519Kyber768 hybrid share + classic X25519 share)
	// Share 1: GREASE (group: RandomGREASE(), len: 1, data: 0x00) -> 2 + 2 + 1 = 5 bytes
	// Share 2: Post-Quantum Hybrid (group: GroupX25519Kyber768, len: 1216, data: 1184B Kyber + 32B X25519) -> 2 + 2 + 1216 = 1220 bytes
	// Share 3: Classic X25519 (group: GroupX25519, len: 32, data: clientPub) -> 2 + 2 + 32 = 36 bytes
	totalSharesLen := 5 + 1220 + 36
	keyShareData := make([]byte, 2+totalSharesLen)
	binary.BigEndian.PutUint16(keyShareData[0:2], uint16(totalSharesLen))
	off := 2

	// Share 1: GREASE
	greaseShareGroup := RandomGREASE()
	binary.BigEndian.PutUint16(keyShareData[off:off+2], greaseShareGroup)
	binary.BigEndian.PutUint16(keyShareData[off+2:off+4], 1)
	keyShareData[off+4] = 0x00
	off += 5

	// Share 2: Kyber768 Hybrid
	binary.BigEndian.PutUint16(keyShareData[off:off+2], GroupX25519Kyber768)
	binary.BigEndian.PutUint16(keyShareData[off+2:off+4], Kyber768HybridShareLen)
	off += 4
	kyberPart := RandomBytes(Kyber768PubKeyLen)
	copy(keyShareData[off:off+Kyber768PubKeyLen], kyberPart)
	copy(keyShareData[off+Kyber768PubKeyLen:off+Kyber768HybridShareLen], clientPub)
	off += Kyber768HybridShareLen

	// Share 3: Classic X25519
	binary.BigEndian.PutUint16(keyShareData[off:off+2], GroupX25519)
	binary.BigEndian.PutUint16(keyShareData[off+2:off+4], uint16(len(clientPub)))
	off += 4
	copy(keyShareData[off:off+len(clientPub)], clientPub)
	off += len(clientPub)

	writeExtension(&extensions, ExtKeyShare, keyShareData)

	// 13. PSK Key Exchange Modes (RFC 8446)
	writeExtension(&extensions, ExtPSKKeyExchangeModes, []byte{0x01, 0x01})

	// 14. Supported Versions (GREASE, TLS 1.3, TLS 1.2)
	greaseVer := RandomGREASE()
	verData := []byte{
		0x06,
		byte(greaseVer >> 8), byte(greaseVer & 0xff),
		0x03, 0x04, // TLS 1.3
		0x03, 0x03, // TLS 1.2
	}
	writeExtension(&extensions, ExtSupportedVersions, verData)

	// 15. Compress Certificate (Brotli - RFC 8879)
	writeExtension(&extensions, ExtCompressCertificate, []byte{0x02, 0x00, 0x02})

	// 16. Application Settings (ALPS for h2)
	writeExtension(&extensions, ExtApplicationSettings, []byte{0x00, 0x03, 0x02, 'h', '2'})

	// 17. Dynamic Padding Extension (clamped to [16, 256])
	if paddingLen < 16 {
		paddingLen = 16
	}
	if paddingLen > 256 {
		paddingLen = 256
	}
	padBytes := make([]byte, paddingLen)
	writeExtension(&extensions, ExtPadding, padBytes)

	// 18. GREASE Extension 2 (RFC 8701)
	greaseExt2 := RandomGREASE()
	writeExtension(&extensions, greaseExt2, []byte{0x00})

	// Assemble ClientHello body
	var body bytes.Buffer
	// Client Version 0x0303 (TLS 1.2)
	body.Write([]byte{0x03, 0x03})
	// Random 32 bytes
	body.Write(RandomBytes(32))
	// Session ID length and data
	body.WriteByte(byte(len(sessionID)))
	body.Write(sessionID)

	// Cipher Suites (16 suites matching Chrome JA3/JA4 fingerprint with leading GREASE)
	greaseCipher := RandomGREASE()
	ciphers := []byte{
		0x00, 0x20, // 32 bytes = 16 cipher suites
		byte(greaseCipher >> 8), byte(greaseCipher & 0xff),
		0x13, 0x01, // TLS_AES_128_GCM_SHA256
		0x13, 0x02, // TLS_AES_256_GCM_SHA384
		0x13, 0x03, // TLS_CHACHA20_POLY1305_SHA256
		0xc0, 0x2b, // ECDHE-ECDSA-AES128-GCM-SHA256
		0xc0, 0x2f, // ECDHE-RSA-AES128-GCM-SHA256
		0xc0, 0x2c, // ECDHE-ECDSA-AES256-GCM-SHA384
		0xc0, 0x30, // ECDHE-RSA-AES256-GCM-SHA384
		0xcc, 0xa9, // ECDHE-ECDSA-CHACHA20-POLY1305
		0xcc, 0xa8, // ECDHE-RSA-CHACHA20-POLY1305
		0xc0, 0x13, // ECDHE-RSA-AES128-SHA
		0xc0, 0x14, // ECDHE-RSA-AES256-SHA
		0x00, 0x9c, // RSA-AES128-GCM-SHA256
		0x00, 0x9d, // RSA-AES256-GCM-SHA384
		0x00, 0x2f, // RSA-AES128-SHA
		0x00, 0x35, // RSA-AES256-SHA
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
	if len(data) > 0 {
		buf.Write(data)
	}
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
					if shareOff+keyLen <= len(shares) {
						if group == GroupX25519 && keyLen == 32 {
							parsed.ClientPub = shares[shareOff : shareOff+keyLen]
							break
						} else if (group == GroupX25519Kyber768 || group == GroupX25519MLKEM768) && keyLen == Kyber768HybridShareLen {
							if parsed.ClientPub == nil {
								// Extract trailing 32 bytes X25519 public key from the 1216B hybrid share
								parsed.ClientPub = shares[shareOff+Kyber768PubKeyLen : shareOff+keyLen]
							}
						}
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
