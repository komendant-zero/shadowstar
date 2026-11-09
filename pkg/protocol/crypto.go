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
	computed := ComputeAuthTag(key, data)
	if len(computed) < len(expectedTag) {
		return false
	}
	return hmac.Equal(computed[:len(expectedTag)], expectedTag)
}

// RandomBytes generates n cryptographically secure random bytes
func RandomBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = io.ReadFull(rand.Reader, b)
	return b
}
