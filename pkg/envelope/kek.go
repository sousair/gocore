// Package envelope implements ADR-012 envelope encryption: each field value is
// encrypted with a per-record DEK (AES-256-GCM); DEKs are wrapped by a KEK
// held by a KEKProvider. The v0 provider reads the KEK from an environment
// variable; future providers (e.g. OpenBao-backed) satisfy the same interface
// with zero call-site changes.
//
// # AAD (Additional Authenticated Data)
//
// Both KEKProvider and FieldCipher accept a WithAAD option that binds the
// ciphertext to contextual identity (e.g. record ID + field name). Decryption
// must present the identical AAD or fail. This prevents a ciphertext from
// being replayed into a different field or record without detection.
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
)

// options holds functional-option state shared by KEKProvider and FieldCipher
// operations.
type options struct {
	aad []byte
}

// Option configures encryption or decryption behaviour.
type Option func(*options)

// WithAAD binds the ciphertext to contextual identity (e.g. a record ID or
// field name). Decryption must present the same AAD or the AEAD open will
// fail. Passing nil or omitting the option uses no AAD.
func WithAAD(aad []byte) Option {
	return func(o *options) {
		o.aad = aad
	}
}

func applyOptions(opts []Option) *options {
	o := &options{}
	for _, fn := range opts {
		fn(o)
	}
	return o
}

// KEKProvider wraps and unwraps DEKs using a Key Encryption Key.
type KEKProvider interface {
	// WrapDEK encrypts dek with the KEK. The optional WithAAD option binds the
	// wrapped ciphertext to a context; UnwrapDEK must receive the same AAD.
	WrapDEK(dek []byte, opts ...Option) ([]byte, error)

	// UnwrapDEK decrypts a previously wrapped DEK. The AAD passed here must
	// match the AAD used during WrapDEK, or decryption will fail.
	UnwrapDEK(wrapped []byte, opts ...Option) ([]byte, error)
}

type envKEKProvider struct{ aead gocipher.AEAD }

// NewEnvKEKProvider constructs a KEKProvider whose KEK is read from the
// environment variable named by envVar. The value must be a base64-encoded
// 32-byte key (AES-256). Returns an error if envVar is unset, not valid
// base64, or not exactly 32 bytes after decoding.
func NewEnvKEKProvider(envVar string) (KEKProvider, error) {
	raw := os.Getenv(envVar)
	if raw == "" {
		return nil, fmt.Errorf("envelope: %s is unset", envVar)
	}
	kek, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("envelope: %s is not valid base64: %w", envVar, err)
	}
	if len(kek) != 32 {
		return nil, fmt.Errorf("envelope: %s must decode to 32 bytes for AES-256, got %d", envVar, len(kek))
	}
	aead, err := newAEAD(kek)
	if err != nil {
		return nil, err
	}
	return &envKEKProvider{aead: aead}, nil
}

func (p *envKEKProvider) WrapDEK(dek []byte, opts ...Option) ([]byte, error) {
	o := applyOptions(opts)
	return seal(p.aead, dek, o.aad)
}

func (p *envKEKProvider) UnwrapDEK(wrapped []byte, opts ...Option) ([]byte, error) {
	o := applyOptions(opts)
	return open(p.aead, wrapped, o.aad)
}

// newAEAD creates an AES-256-GCM AEAD from a 32-byte key.
func newAEAD(key []byte) (gocipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("envelope: %w", err)
	}
	return gocipher.NewGCM(block)
}

// seal encrypts plaintext with aad as additional authenticated data. The
// random nonce is prepended to the ciphertext so the result is self-contained.
func seal(aead gocipher.AEAD, plaintext, aad []byte) ([]byte, error) {
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("envelope: nonce: %w", err)
	}
	return aead.Seal(nonce, nonce, plaintext, aad), nil
}

// open decrypts a sealed value produced by seal. aad must match what was
// passed to seal, or decryption will fail.
func open(aead gocipher.AEAD, sealed, aad []byte) ([]byte, error) {
	if len(sealed) < aead.NonceSize() {
		return nil, errors.New("envelope: sealed value too short")
	}
	nonce, ct := sealed[:aead.NonceSize()], sealed[aead.NonceSize():]
	pt, err := aead.Open(nil, nonce, ct, aad)
	if err != nil {
		return nil, fmt.Errorf("envelope: open: %w", err)
	}
	return pt, nil
}
