package protocol

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
)

const (
	KeySize   = 32
	NonceSize = 12
	TagSize   = 16
)

// HKDFExtract performs HKDF-Extract using HMAC-SHA256
func HKDFExtract(salt, secret []byte) []byte {
	if len(salt) == 0 {
		salt = make([]byte, sha256.Size)
	}
	h := hmac.New(sha256.New, salt)
	h.Write(secret)
	return h.Sum(nil)
}

// HKDFExpand performs HKDF-Expand using HMAC-SHA256
func HKDFExpand(prk, info []byte, outLen int) ([]byte, error) {
	out := make([]byte, outLen)
	h := hmac.New(sha256.New, prk)
	var prev []byte
	blockCount := (outLen + sha256.Size - 1) / sha256.Size

	for i := 1; i <= blockCount; i++ {
		h.Reset()
		h.Write(prev)
		h.Write(info)
		h.Write([]byte{byte(i)})
		prev = h.Sum(nil)

		offset := (i - 1) * sha256.Size
		copyLen := sha256.Size
		if offset+copyLen > outLen {
			copyLen = outLen - offset
		}
		copy(out[offset:offset+copyLen], prev[:copyLen])
	}
	return out, nil
}

// DeriveSessionKeys derives bidirectional encryption keys and base IV salts from shared secret and salt
func DeriveSessionKeys(sharedSecret, salt []byte) (clientKey, serverKey, clientIV, serverIV []byte, err error) {
	prk := HKDFExtract(salt, sharedSecret)
	keyMaterial, err := HKDFExpand(prk, []byte("shadowstar-session-v1"), (KeySize+NonceSize)*2)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	clientKey = keyMaterial[0:KeySize]
	clientIV = keyMaterial[KeySize : KeySize+NonceSize]
	serverKey = keyMaterial[KeySize+NonceSize : 2*KeySize+NonceSize]
	serverIV = keyMaterial[2*KeySize+NonceSize : 2*KeySize+2*NonceSize]
	return clientKey, serverKey, clientIV, serverIV, nil
}

// GenerateX25519KeyPair generates a new ephemeral X25519 keypair
func GenerateX25519KeyPair() (*ecdh.PrivateKey, []byte, error) {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	pub := priv.PublicKey().Bytes()
	return priv, pub, nil
}

// ComputeECDH computes the shared secret given a private key and the peer's public key
func ComputeECDH(priv *ecdh.PrivateKey, peerPubBytes []byte) ([]byte, error) {
	peerPub, err := ecdh.X25519().NewPublicKey(peerPubBytes)
	if err != nil {
		return nil, fmt.Errorf("invalid peer public key: %w", err)
	}
	return priv.ECDH(peerPub)
}

// CreateGCMCipher initializes an AES-256-GCM AEAD cipher
func CreateGCMCipher(key []byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, errors.New("key must be 32 bytes for AES-256")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// ComputeAuthTag calculates an HMAC-SHA256 authentication tag
func ComputeAuthTag(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

// VerifyAuthTag verifies an authentication tag in constant time
func VerifyAuthTag(key, data, expectedTag []byte) bool {
	if len(expectedTag) < 16 {
		return false
	}
	computed := ComputeAuthTag(key, data)
	if len(computed) < len(expectedTag) {
		return false
	}
	return hmac.Equal(computed[:len(expectedTag)], expectedTag)
}

// NormalizeMasterKey normalizes raw master key strings into a secure 32-byte key.
// It also provides a legacyKey fallback so connections to previously deployed servers
// (which truncated 64-character hex keys without decoding) continue to work seamlessly.
func NormalizeMasterKey(rawKey string) (primaryKey []byte, legacyKey []byte) {
	trimmed := []byte(nil)
	cleanStr := ""
	for i := 0; i < len(rawKey); i++ {
		c := rawKey[i]
		if c > ' ' && c <= '~' {
			cleanStr += string(c)
		}
	}
	trimmed = []byte(cleanStr)
	if len(trimmed) == 0 {
		return make([]byte, KeySize), nil
	}

	// Case 1: 64-char Hex String (256-bit key encoded as hex)
	if len(cleanStr) == 64 {
		decoded := make([]byte, 32)
		isHex := true
		for i := 0; i < 32; i++ {
			hi := fromHexChar(cleanStr[2*i])
			lo := fromHexChar(cleanStr[2*i+1])
			if hi == 255 || lo == 255 {
				isHex = false
				break
			}
			decoded[i] = (hi << 4) | lo
		}
		if isHex {
			// primaryKey is the true cryptographically decoded 256-bit key
			// legacyKey is the first 32 ASCII characters (for backward compatibility with old servers)
			return decoded, trimmed[:32]
		}
	}

	// Case 2: Exactly 32 bytes
	if len(trimmed) == 32 {
		return trimmed, nil
	}

	// Case 3: Less than 32 bytes - safely derive 32 bytes via HKDF
	if len(trimmed) < 32 {
		return HKDFExtract(nil, trimmed), nil
	}

	// Case 4: Long string
	return trimmed[:32], nil
}

func fromHexChar(c byte) byte {
	switch {
	case '0' <= c && c <= '9':
		return c - '0'
	case 'a' <= c && c <= 'f':
		return c - 'a' + 10
	case 'A' <= c && c <= 'F':
		return c - 'A' + 10
	}
	return 255
}

// RandomBytes generates n cryptographically secure random bytes
func RandomBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = io.ReadFull(rand.Reader, b)
	return b
}
