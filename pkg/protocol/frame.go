package protocol

import (
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"
	"sync"
)

const (
	FrameOpen  byte = 0x01
	FrameData  byte = 0x02
	FrameClose byte = 0x03
	FramePing  byte = 0x04
	FramePong  byte = 0x05

	MaxPayloadSize = 32768
	MaxPaddingSize = 256
	MinPaddingSize = 16
)

// Frame represents a multiplexed, padded protocol packet
type Frame struct {
	StreamID byte
	StreamNum uint32
	Type      byte
	Payload   []byte
}

// SecureFrameConn wraps a net.Conn with AEAD frame encryption, padding, and replay defense
type SecureFrameConn struct {
	conn      io.ReadWriter
	encCipher cipher.AEAD
	decCipher cipher.AEAD
	encNonce  uint64
	decNonce  uint64
	encSalt   []byte
	decSalt   []byte
	muWrite   sync.Mutex
	muRead    sync.Mutex
	closed    bool
}

// NewSecureFrameConn creates a new secure bidirectional frame connection
func NewSecureFrameConn(rw io.ReadWriter, encKey, encSalt, decKey, decSalt []byte) (*SecureFrameConn, error) {
	encCipher, err := CreateGCMCipher(encKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create enc cipher: %w", err)
	}

	decCipher, err := CreateGCMCipher(decKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create dec cipher: %w", err)
	}

	return &SecureFrameConn{
		conn:      rw,
		encCipher: encCipher,
		decCipher: decCipher,
		encSalt:   encSalt,
		decSalt:   decSalt,
	}, nil
}

func (s *SecureFrameConn) getNonce(baseSalt []byte, seq uint64) []byte {
	nonce := make([]byte, NonceSize)
	copy(nonce, baseSalt[:NonceSize])
	binary.BigEndian.PutUint64(nonce[NonceSize-8:], binary.BigEndian.Uint64(nonce[NonceSize-8:])^seq)
	return nonce
}

// WriteFrame encrypts and writes a frame with random dynamic morphing padding
func (s *SecureFrameConn) WriteFrame(streamID uint32, frameType byte, payload []byte) error {
	s.muWrite.Lock()
	defer s.muWrite.Unlock()

	if len(payload) > MaxPayloadSize {
		return errors.New("payload exceeds maximum size")
	}

	// Calculate random padding length (16 - 128 bytes)
	padBig, _ := rand.Int(rand.Reader, big.NewInt(MaxPaddingSize-MinPaddingSize))
	paddingLen := int(padBig.Int64()) + MinPaddingSize

	payloadLen := len(payload)
	// Body unencrypted: StreamID(4) + Type(1) + PayloadLen(2) + PaddingLen(2) + Payload + Padding
	bodyPlainLen := 4 + 1 + 2 + 2 + payloadLen + paddingLen
	bodyPlain := make([]byte, bodyPlainLen)

	binary.BigEndian.PutUint32(bodyPlain[0:4], streamID)
	bodyPlain[4] = frameType
	binary.BigEndian.PutUint16(bodyPlain[5:7], uint16(payloadLen))
	binary.BigEndian.PutUint16(bodyPlain[7:9], uint16(paddingLen))
	copy(bodyPlain[9:9+payloadLen], payload)

	// Fill random padding
	if paddingLen > 0 {
		_, _ = io.ReadFull(rand.Reader, bodyPlain[9+payloadLen:])
	}

	// 1. Encrypt Total Body Length (2 bytes) + Tag (16 bytes)
	lenPlain := make([]byte, 2)
	totalEncLen := bodyPlainLen + TagSize
	binary.BigEndian.PutUint16(lenPlain, uint16(totalEncLen))

	headerNonce := s.getNonce(s.encSalt, s.encNonce)
	s.encNonce++
	lenEnc := s.encCipher.Seal(nil, headerNonce, lenPlain, nil)

	// 2. Encrypt Body
	bodyNonce := s.getNonce(s.encSalt, s.encNonce)
	s.encNonce++
	bodyEnc := s.encCipher.Seal(nil, bodyNonce, bodyPlain, nil)

	// Write length + body
	if _, err := s.conn.Write(lenEnc); err != nil {
		return err
	}
	if _, err := s.conn.Write(bodyEnc); err != nil {
		return err
	}

	return nil
}

// ReadFrame reads and decrypts the next frame, stripping dynamic padding
func (s *SecureFrameConn) ReadFrame() (streamID uint32, frameType byte, payload []byte, err error) {
	s.muRead.Lock()
	defer s.muRead.Unlock()

	// 1. Read encrypted 2-byte length + 16-byte tag
	headerBuf := make([]byte, 2+TagSize)
	if _, err := io.ReadFull(s.conn, headerBuf); err != nil {
		return 0, 0, nil, err
	}

	headerNonce := s.getNonce(s.decSalt, s.decNonce)
	s.decNonce++
	lenPlain, err := s.decCipher.Open(nil, headerNonce, headerBuf, nil)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("header decrypt auth failed: %w", err)
	}

	bodyEncLen := int(binary.BigEndian.Uint16(lenPlain))
	if bodyEncLen < 9+TagSize || bodyEncLen > MaxPayloadSize+MaxPaddingSize+9+TagSize {
		return 0, 0, nil, errors.New("frame length out of bounds")
	}

	// 2. Read encrypted body
	bodyEnc := make([]byte, bodyEncLen)
	if _, err := io.ReadFull(s.conn, bodyEnc); err != nil {
		return 0, 0, nil, err
	}

	bodyNonce := s.getNonce(s.decSalt, s.decNonce)
	s.decNonce++
	bodyPlain, err := s.decCipher.Open(nil, bodyNonce, bodyEnc, nil)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("body decrypt auth failed: %w", err)
	}

	streamID = binary.BigEndian.Uint32(bodyPlain[0:4])
	frameType = bodyPlain[4]
	payloadLen := int(binary.BigEndian.Uint16(bodyPlain[5:7]))
	paddingLen := int(binary.BigEndian.Uint16(bodyPlain[7:9]))

	if 9+payloadLen+paddingLen > len(bodyPlain) {
		return 0, 0, nil, errors.New("malformed frame payload bounds")
	}

	payload = make([]byte, payloadLen)
	copy(payload, bodyPlain[9:9+payloadLen])

	return streamID, frameType, payload, nil
}
