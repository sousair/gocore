package envelope

import (
	"crypto/aes"
	gocipher "crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// KeyWrapper encrypts ("wraps") and decrypts data keys with a long-lived
// wrapping key, so data keys can be stored at rest in wrapped form.
type KeyWrapper interface {
	Wrap(key []byte) ([]byte, error)
	Unwrap(wrapped []byte) ([]byte, error)
}

type envKeyWrapper struct{ aead gocipher.AEAD }

// NewEnvKeyWrapper builds a KeyWrapper from a base64-encoded 32-byte AES-256
// key held in the named environment variable.
func NewEnvKeyWrapper(envVar string) (KeyWrapper, error) {
	raw := os.Getenv(envVar)
	if raw == "" {
		return nil, fmt.Errorf("envelope: %s is unset", envVar)
	}
	return keyWrapperFromBase64(raw, envVar)
}

// NewFileKeyWrapper builds a KeyWrapper from a base64-encoded 32-byte AES-256
// key held in the file at path.
func NewFileKeyWrapper(path string) (KeyWrapper, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("envelope: read key file %s: %w", path, err)
	}
	// mounted secret files commonly end in a trailing newline.
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil, fmt.Errorf("envelope: key file %s is empty", path)
	}
	return keyWrapperFromBase64(trimmed, path)
}

// NewKeyWrapper resolves a KeyWrapper from either a mounted key file or an
// inline env var, preferring the file when fileEnvVar is set.
func NewKeyWrapper(fileEnvVar, valueEnvVar string) (KeyWrapper, error) {
	if path := os.Getenv(fileEnvVar); path != "" {
		return NewFileKeyWrapper(path)
	}
	if os.Getenv(valueEnvVar) != "" {
		return NewEnvKeyWrapper(valueEnvVar)
	}
	return nil, fmt.Errorf("envelope: neither %s nor %s is set", fileEnvVar, valueEnvVar)
}

// keyWrapperFromBase64 decodes raw as a base64 32-byte AES-256 key. source is
// used only for error messages (the env var name or file path).
func keyWrapperFromBase64(raw, source string) (KeyWrapper, error) {
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("envelope: %s is not valid base64: %w", source, err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("envelope: %s must decode to 32 bytes for AES-256, got %d", source, len(key))
	}
	aead, err := newAEAD(key)
	if err != nil {
		return nil, err
	}
	return &envKeyWrapper{aead: aead}, nil
}

func (w *envKeyWrapper) Wrap(key []byte) ([]byte, error)       { return seal(w.aead, key, nil) }
func (w *envKeyWrapper) Unwrap(wrapped []byte) ([]byte, error) { return open(w.aead, wrapped, nil) }

// newAEAD builds an AES-256-GCM AEAD from a 32-byte key.
func newAEAD(key []byte) (gocipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("envelope: %w", err)
	}
	return gocipher.NewGCM(block)
}

// seal encrypts plaintext and prepends the random nonce, returning a
// self-contained value. associatedData is authenticated but not encrypted.
func seal(aead gocipher.AEAD, plaintext, associatedData []byte) ([]byte, error) {
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("envelope: nonce: %w", err)
	}
	return aead.Seal(nonce, nonce, plaintext, associatedData), nil
}

// open reverses seal. associatedData must match what seal received.
func open(aead gocipher.AEAD, sealed, associatedData []byte) ([]byte, error) {
	if len(sealed) < aead.NonceSize() {
		return nil, errors.New("envelope: sealed value too short")
	}
	nonce, ciphertext := sealed[:aead.NonceSize()], sealed[aead.NonceSize():]
	plaintext, err := aead.Open(nil, nonce, ciphertext, associatedData)
	if err != nil {
		return nil, fmt.Errorf("envelope: open: %w", err)
	}
	return plaintext, nil
}
