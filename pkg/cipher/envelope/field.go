package envelope

import (
	gocipher "crypto/cipher"
	"fmt"
)

// FieldCipher encrypts and decrypts individual values with a 32-byte data key
// (AES-256-GCM). associatedData binds a ciphertext to its context (e.g. a
// record id or field name); Decrypt must receive the same bytes Encrypt used.
type FieldCipher struct{ aead gocipher.AEAD }

// NewFieldCipher builds a FieldCipher from a 32-byte data key.
func NewFieldCipher(key []byte) (*FieldCipher, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("envelope: data key must be 32 bytes for AES-256-GCM, got %d", len(key))
	}
	aead, err := newAEAD(key)
	if err != nil {
		return nil, err
	}
	return &FieldCipher{aead: aead}, nil
}

func (f *FieldCipher) Encrypt(plaintext, associatedData []byte) ([]byte, error) {
	return seal(f.aead, plaintext, associatedData)
}

func (f *FieldCipher) Decrypt(ciphertext, associatedData []byte) ([]byte, error) {
	return open(f.aead, ciphertext, associatedData)
}
