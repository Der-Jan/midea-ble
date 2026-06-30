package proto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"

	"github.com/pion/dtls/v3/pkg/crypto/ccm"
	"golang.org/x/crypto/hkdf"
)

// 协议常量
var saltInfo = []byte("midea_bleapp") // 实为 HKDF 的 info（salt 为空）

const (
	rootKeyLen    = 16
	sessionKeyLen = 16
)

// DeriveRootKey: rootKey = HKDF-SHA256(ikm=advertisData, salt=空, info="midea_bleapp", len=16)
// 注意：源码里 "midea_bleapp" 是 info 不是 salt；salt 为空（HMAC 下等价零盐）。
func DeriveRootKey(advertisData []byte) ([]byte, error) {
	r := hkdf.New(sha256.New, advertisData, nil /*salt 空*/, saltInfo)
	key := make([]byte, rootKeyLen)
	if _, err := io.ReadFull(r, key); err != nil {
		return nil, err
	}
	return key, nil
}

// CreateKeypair 生成 P-256 keypair，返回 (priv 32B, pub64 = X||Y)。
func CreateKeypair() (priv []byte, pub64 []byte, err error) {
	k, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	pubFull := k.PublicKey().Bytes() // 65B: 0x04 || X || Y
	return k.Bytes(), pubFull[1:], nil
}

// DeriveSessionKey: sessionKey = SHA-256( P-256_ECDH_X(myPri, peerPub64) )[:16]
func DeriveSessionKey(myPri, peerPub64 []byte) ([]byte, error) {
	peerFull := append([]byte{0x04}, peerPub64...)
	peerPub, err := ecdh.P256().NewPublicKey(peerFull)
	if err != nil {
		return nil, fmt.Errorf("peer pubkey invalid: %w", err)
	}
	priv, err := ecdh.P256().NewPrivateKey(myPri)
	if err != nil {
		return nil, err
	}
	sharedX, err := priv.ECDH(peerPub) // 32B = 共享点 X 坐标
	if err != nil {
		return nil, err
	}
	h := sha256.Sum256(sharedX)
	return h[:sessionKeyLen], nil
}

func newCCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	c, err := ccm.NewCCM(block, 8 /*tag*/, 8 /*nonce*/)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// CipherMsg: nonce(8) || ciphertext || tag(8)，AES-128-CCM。
func CipherMsg(key, plaintext []byte) ([]byte, error) {
	if len(key) != 16 {
		return nil, fmt.Errorf("key must be 16 bytes, got %d", len(key))
	}
	nonce := make([]byte, 8)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	c, err := newCCM(key)
	if err != nil {
		return nil, err
	}
	ctTag := c.Seal(nil, nonce, plaintext, nil) // ct || tag
	out := make([]byte, 0, 8+len(ctTag))
	out = append(out, nonce...)
	out = append(out, ctTag...)
	return out, nil
}

// DecipherMsg 解开 nonce||ct||tag。
func DecipherMsg(key, blob []byte) ([]byte, error) {
	if len(key) != 16 {
		return nil, fmt.Errorf("key must be 16 bytes")
	}
	if len(blob) < 16 {
		return nil, fmt.Errorf("cipher blob too short: %x", blob)
	}
	nonce := blob[:8]
	ctTag := blob[8:]
	c, err := newCCM(key)
	if err != nil {
		return nil, err
	}
	return c.Open(nil, nonce, ctTag, nil)
}
