package envelope

import (
	gocipher "crypto/cipher"
	"fmt"
)

// FieldCipher encrypts and decrypts individual field values using a DEK
// (data-encryption key). The DEK must be exactly 32 bytes (AES-256-GCM).
type FieldCipher struct{ aead gocipher.AEAD }

// NewFieldCipher constructs a FieldCipher from a 32-byte DEK. Returns an
// error if the DEK is not exactly 32 bytes.
func NewFieldCipher(dek []byte) (*FieldCipher, error) {
	if len(dek) != 32 {
		return nil, fmt.Errorf("envelope: DEK must be 32 bytes for AES-256-GCM, got %d", len(dek))
	}
	aead, err := newAEAD(dek)
	if err != nil {
		return nil, err
	}
	return &FieldCipher{aead: aead}, nil
}

// Encrypt encrypts plaintext using AES-256-GCM. A random nonce is prepended
// to the ciphertext. The optional WithAAD option binds the ciphertext to a
// context; Decrypt must receive the same AAD.
func (f *FieldCipher) Encrypt(plaintext []byte, opts ...Option) ([]byte, error) {
	o := applyOptions(opts)
	return seal(f.aead, plaintext, o.aad)
}

// Decrypt decrypts a ciphertext produced by Encrypt. The AAD passed here must
// match the AAD used during Encrypt, or decryption will fail.
func (f *FieldCipher) Decrypt(ciphertext []byte, opts ...Option) ([]byte, error) {
	o := applyOptions(opts)
	return open(f.aead, ciphertext, o.aad)
}
