package jwt

import (
	"errors"
	"time"

	jwtgo "github.com/dgrijalva/jwt-go"
)

type (
	JWT[T any] interface {
		Generate(payload *T) (string, error)
		Validate(token string) (*T, error)
	}

	tokenPayload[T any] struct {
		jwtgo.StandardClaims
		Payload *T `json:"payload"`
	}
)

type jwt[T any] struct {
	secret         []byte
	expirationInMs int
}

var _ JWT[any] = (*jwt[any])(nil)

var ErrInvalidToken = errors.New("invalid token")

func New[T any](secret string, expirationInMs int) JWT[T] {
	return &jwt[T]{
		secret: []byte(secret),
	}
}

func (j *jwt[T]) Generate(payload *T) (string, error) {
	token := jwtgo.NewWithClaims(jwtgo.SigningMethodHS256, tokenPayload[T]{
		Payload: payload,
		StandardClaims: jwtgo.StandardClaims{
			// TODO: Check this
			ExpiresAt: jwtgo.TimeFunc().Add(time.Millisecond * time.Duration(j.expirationInMs)).Unix(),
		},
	})

	return token.SignedString(j.secret)
}

func (j *jwt[T]) Validate(token string) (*T, error) {
	claims := &tokenPayload[T]{}

	t, err := jwtgo.ParseWithClaims(token, claims, func(token *jwtgo.Token) (interface{}, error) {
		return j.secret, nil
	})
	if err != nil {
		return nil, err
	}

	if !t.Valid {
		return nil, ErrInvalidToken
	}

	return claims.Payload, nil
}
