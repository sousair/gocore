package envelope_test

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"testing"

	"github.com/sousair/gocore/pkg/envelope"
)

// newKEKEnv returns a base64-encoded random 32-byte key suitable for use as
// an env-var KEK and sets it on the named env var for the duration of the test.
func newKEKEnv(t *testing.T, envVar string) {
	t.Helper()
	kek := make([]byte, 32)
	if _, err := rand.Read(kek); err != nil {
		t.Fatal(err)
	}
	t.Setenv(envVar, base64.StdEncoding.EncodeToString(kek))
}

// --- EnvKEKProvider tests ---

func TestEnvKEKProvider(t *testing.T) {
	const envVar = "GOCORE_TEST_KEK"

	t.Run("given a valid env KEK/when a DEK is wrapped and unwrapped/then the roundtrip is lossless", func(t *testing.T) {
		newKEKEnv(t, envVar)
		p, err := envelope.NewEnvKEKProvider(envVar)
		if err != nil {
			t.Fatal(err)
		}
		dek := []byte("0123456789abcdef0123456789abcdef")
		wrapped, err := p.WrapDEK(dek)
		if err != nil {
			t.Fatal(err)
		}
		got, err := p.UnwrapDEK(wrapped)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, dek) {
			t.Errorf("roundtrip mismatch: got %x, want %x", got, dek)
		}
	})

	t.Run("given a wrapped DEK/when the ciphertext is tampered/then unwrap fails", func(t *testing.T) {
		newKEKEnv(t, envVar)
		p, _ := envelope.NewEnvKEKProvider(envVar)
		wrapped, _ := p.WrapDEK([]byte("0123456789abcdef0123456789abcdef"))
		wrapped[len(wrapped)-1] ^= 0xFF
		if _, err := p.UnwrapDEK(wrapped); err == nil {
			t.Error("want error on tampered ciphertext, got nil")
		}
	})

	t.Run("given the env var is unset/when the provider is constructed/then it errors", func(t *testing.T) {
		t.Setenv(envVar, "")
		if _, err := envelope.NewEnvKEKProvider(envVar); err == nil {
			t.Error("want error for unset env var, got nil")
		}
	})

	t.Run("given the env var is not base64/when the provider is constructed/then it errors", func(t *testing.T) {
		t.Setenv(envVar, "not-base64!!!")
		if _, err := envelope.NewEnvKEKProvider(envVar); err == nil {
			t.Error("want error for non-base64 env var, got nil")
		}
	})

	t.Run("given the env var decodes to the wrong length/when the provider is constructed/then it errors", func(t *testing.T) {
		t.Setenv(envVar, base64.StdEncoding.EncodeToString(make([]byte, 16)))
		if _, err := envelope.NewEnvKEKProvider(envVar); err == nil {
			t.Error("want error for 16-byte KEK, got nil")
		}
	})
}

// --- FieldCipher tests ---

func TestFieldCipher(t *testing.T) {
	t.Run("given a DEK/when a field value is encrypted and decrypted/then the roundtrip is lossless", func(t *testing.T) {
		dek := make([]byte, 32)
		if _, err := rand.Read(dek); err != nil {
			t.Fatal(err)
		}
		fc, err := envelope.NewFieldCipher(dek)
		if err != nil {
			t.Fatal(err)
		}
		ct, err := fc.Encrypt([]byte("rendimento do FII em janeiro"))
		if err != nil {
			t.Fatal(err)
		}
		pt, err := fc.Decrypt(ct)
		if err != nil {
			t.Fatal(err)
		}
		if string(pt) != "rendimento do FII em janeiro" {
			t.Errorf("roundtrip mismatch: %q", pt)
		}
	})

	t.Run("given a DEK that is not 32 bytes/when the field cipher is constructed/then it errors", func(t *testing.T) {
		if _, err := envelope.NewFieldCipher(make([]byte, 16)); err == nil {
			t.Error("want error for 16-byte DEK, got nil")
		}
	})

	// --- AAD tests ---

	t.Run("given matching AAD/when encrypt then decrypt with same AAD/then roundtrip succeeds", func(t *testing.T) {
		dek := make([]byte, 32)
		if _, err := rand.Read(dek); err != nil {
			t.Fatal(err)
		}
		fc, err := envelope.NewFieldCipher(dek)
		if err != nil {
			t.Fatal(err)
		}
		aad := []byte("record-id:abc123 field:salary")
		ct, err := fc.Encrypt([]byte("secret value"), envelope.WithAAD(aad))
		if err != nil {
			t.Fatal(err)
		}
		pt, err := fc.Decrypt(ct, envelope.WithAAD(aad))
		if err != nil {
			t.Fatalf("decrypt with matching AAD failed: %v", err)
		}
		if string(pt) != "secret value" {
			t.Errorf("roundtrip mismatch: %q", pt)
		}
	})

	t.Run("given mismatched AAD/when decrypt is called with different AAD/then it fails", func(t *testing.T) {
		dek := make([]byte, 32)
		if _, err := rand.Read(dek); err != nil {
			t.Fatal(err)
		}
		fc, _ := envelope.NewFieldCipher(dek)
		ct, _ := fc.Encrypt([]byte("secret"), envelope.WithAAD([]byte("record-id:abc123")))
		if _, err := fc.Decrypt(ct, envelope.WithAAD([]byte("record-id:DIFFERENT"))); err == nil {
			t.Error("want error when AAD differs, got nil")
		}
	})

	t.Run("given AAD used at encrypt/when decrypt is called without AAD/then it fails", func(t *testing.T) {
		dek := make([]byte, 32)
		if _, err := rand.Read(dek); err != nil {
			t.Fatal(err)
		}
		fc, _ := envelope.NewFieldCipher(dek)
		ct, _ := fc.Encrypt([]byte("secret"), envelope.WithAAD([]byte("context")))
		if _, err := fc.Decrypt(ct); err == nil {
			t.Error("want error when AAD omitted but was used at encrypt, got nil")
		}
	})
}

// --- KEKProvider AAD tests ---

func TestEnvKEKProvider_AAD(t *testing.T) {
	const envVar = "GOCORE_TEST_KEK_AAD"

	t.Run("given matching AAD/when WrapDEK then UnwrapDEK with same AAD/then roundtrip succeeds", func(t *testing.T) {
		newKEKEnv(t, envVar)
		p, err := envelope.NewEnvKEKProvider(envVar)
		if err != nil {
			t.Fatal(err)
		}
		dek := make([]byte, 32)
		if _, err := rand.Read(dek); err != nil {
			t.Fatal(err)
		}
		aad := []byte("dek-id:xyz field:ssn")
		wrapped, err := p.WrapDEK(dek, envelope.WithAAD(aad))
		if err != nil {
			t.Fatal(err)
		}
		got, err := p.UnwrapDEK(wrapped, envelope.WithAAD(aad))
		if err != nil {
			t.Fatalf("unwrap with matching AAD failed: %v", err)
		}
		if !bytes.Equal(got, dek) {
			t.Errorf("roundtrip mismatch")
		}
	})

	t.Run("given AAD used at wrap/when unwrap is called with different AAD/then it fails", func(t *testing.T) {
		newKEKEnv(t, envVar)
		p, _ := envelope.NewEnvKEKProvider(envVar)
		dek := make([]byte, 32)
		rand.Read(dek) //nolint:errcheck
		wrapped, _ := p.WrapDEK(dek, envelope.WithAAD([]byte("original-context")))
		if _, err := p.UnwrapDEK(wrapped, envelope.WithAAD([]byte("tampered-context"))); err == nil {
			t.Error("want error when AAD differs, got nil")
		}
	})
}
