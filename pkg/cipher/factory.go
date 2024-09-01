package cipher

import "golang.org/x/crypto/bcrypt"

type Cipher interface {
	Encrypt(string) (string, error)
	Compare(plainText, cipherText string) error
}

type bcryptCipher struct {
	cost int
}

var _ Cipher = (*bcryptCipher)(nil)

func NewBcryptCipher(hashCost int) Cipher {
	return &bcryptCipher{
		cost: hashCost,
	}
}

func (b *bcryptCipher) Encrypt(plainText string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plainText), b.cost)
	if err != nil {
		return "", err
	}

	return string(hash), nil
}

func (b *bcryptCipher) Compare(plainText, cipherText string) error {
	return bcrypt.CompareHashAndPassword([]byte(cipherText), []byte(plainText))
}
