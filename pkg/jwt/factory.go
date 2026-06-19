package jwt

import (
	"errors"
	"time"

	jwtv5 "github.com/golang-jwt/jwt/v5"
)

type (
	JWT[T any] interface {
		Generate(payload *T) (string, error)
		Validate(token string) (*T, error)
	}

	tokenPayload[T any] struct {
		jwtv5.RegisteredClaims
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
		secret:         []byte(secret),
		expirationInMs: expirationInMs,
	}
}

func (j *jwt[T]) Generate(payload *T) (string, error) {
	token := jwtv5.NewWithClaims(jwtv5.SigningMethodHS256, tokenPayload[T]{
		Payload: payload,
		RegisteredClaims: jwtv5.RegisteredClaims{
			ExpiresAt: jwtv5.NewNumericDate(time.Now().Add(time.Duration(j.expirationInMs) * time.Millisecond)),
		},
	})

	return token.SignedString(j.secret)
}

func (j *jwt[T]) Validate(token string) (*T, error) {
	claims := &tokenPayload[T]{}

	t, err := jwtv5.ParseWithClaims(token, claims, func(token *jwtv5.Token) (any, error) {
		return j.secret, nil
	}, jwtv5.WithValidMethods([]string{"HS256"}))
	if err != nil || !t.Valid {
		return nil, ErrInvalidToken
	}

	return claims.Payload, nil
}
