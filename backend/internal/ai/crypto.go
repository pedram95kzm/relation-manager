package ai

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
)

// Associated data binds this versioned ciphertext to its intended column.
var keyAAD = []byte("relations/provider_config/encrypted_key/v1")

func encryptKey(master []byte, secret string) ([]byte, error) {
	if len(master) != 32 {
		return nil, errors.New("invalid encryption configuration")
	}
	block, err := aes.NewCipher(master)
	if err != nil {
		return nil, errors.New("invalid encryption configuration")
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	result := append([]byte{1}, nonce...)
	return gcm.Seal(result, nonce, []byte(secret), keyAAD), nil
}

func decryptKey(master, encrypted []byte) (string, error) {
	if len(master) != 32 {
		return "", errors.New("invalid encryption configuration")
	}
	block, err := aes.NewCipher(master)
	if err != nil {
		return "", errors.New("invalid encryption configuration")
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(encrypted) < 1+gcm.NonceSize()+gcm.Overhead() || encrypted[0] != 1 {
		return "", errors.New("invalid encrypted credential")
	}
	plain, err := gcm.Open(nil, encrypted[1:1+gcm.NonceSize()], encrypted[1+gcm.NonceSize():], keyAAD)
	if err != nil {
		return "", errors.New("invalid encrypted credential")
	}
	return string(plain), nil
}
