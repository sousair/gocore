package envelope_test

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"testing"

	"github.com/sousair/gocore/pkg/cipher/envelope"
)

// setEnvKey sets a random base64 32-byte key on the named env var for the test.
func setEnvKey(t *testing.T, envVar string) {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	t.Setenv(envVar, base64.StdEncoding.EncodeToString(key))
}

func TestEnvKeyWrapper(t *testing.T) {
	const envVar = "GOCORE_TEST_WRAP_KEY"

	t.Run("wrap then unwrap is lossless", func(t *testing.T) {
		setEnvKey(t, envVar)
		w, err := envelope.NewEnvKeyWrapper(envVar)
		if err != nil {
			t.Fatal(err)
		}
		key := []byte("0123456789abcdef0123456789abcdef")
		wrapped, err := w.Wrap(key)
		if err != nil {
			t.Fatal(err)
		}
		got, err := w.Unwrap(wrapped)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, key) {
			t.Errorf("roundtrip mismatch: got %x, want %x", got, key)
		}
	})

	t.Run("tampered wrapped key fails to unwrap", func(t *testing.T) {
		setEnvKey(t, envVar)
		w, _ := envelope.NewEnvKeyWrapper(envVar)
		wrapped, _ := w.Wrap([]byte("0123456789abcdef0123456789abcdef"))
		wrapped[len(wrapped)-1] ^= 0xFF
		if _, err := w.Unwrap(wrapped); err == nil {
			t.Error("want error on tampered wrapped key, got nil")
		}
	})

	t.Run("unset env var errors", func(t *testing.T) {
		t.Setenv(envVar, "")
		if _, err := envelope.NewEnvKeyWrapper(envVar); err == nil {
			t.Error("want error for unset env var, got nil")
		}
	})

	t.Run("non-base64 env var errors", func(t *testing.T) {
		t.Setenv(envVar, "not-base64!!!")
		if _, err := envelope.NewEnvKeyWrapper(envVar); err == nil {
			t.Error("want error for non-base64 env var, got nil")
		}
	})

	t.Run("wrong-length key errors", func(t *testing.T) {
		t.Setenv(envVar, base64.StdEncoding.EncodeToString(make([]byte, 16)))
		if _, err := envelope.NewEnvKeyWrapper(envVar); err == nil {
			t.Error("want error for 16-byte key, got nil")
		}
	})
}

func TestFieldCipher(t *testing.T) {
	newCipher := func(t *testing.T) *envelope.FieldCipher {
		t.Helper()
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			t.Fatal(err)
		}
		fc, err := envelope.NewFieldCipher(key)
		if err != nil {
			t.Fatal(err)
		}
		return fc
	}

	t.Run("encrypt then decrypt is lossless", func(t *testing.T) {
		fc := newCipher(t)
		ct, err := fc.Encrypt([]byte("rendimento do FII em janeiro"), nil)
		if err != nil {
			t.Fatal(err)
		}
		pt, err := fc.Decrypt(ct, nil)
		if err != nil {
			t.Fatal(err)
		}
		if string(pt) != "rendimento do FII em janeiro" {
			t.Errorf("roundtrip mismatch: %q", pt)
		}
	})

	t.Run("wrong-length key errors", func(t *testing.T) {
		if _, err := envelope.NewFieldCipher(make([]byte, 16)); err == nil {
			t.Error("want error for 16-byte key, got nil")
		}
	})

	t.Run("matching associated data roundtrips", func(t *testing.T) {
		fc := newCipher(t)
		ad := []byte("record-id:abc123 field:salary")
		ct, err := fc.Encrypt([]byte("secret value"), ad)
		if err != nil {
			t.Fatal(err)
		}
		pt, err := fc.Decrypt(ct, ad)
		if err != nil {
			t.Fatalf("decrypt with matching associated data failed: %v", err)
		}
		if string(pt) != "secret value" {
			t.Errorf("roundtrip mismatch: %q", pt)
		}
	})

	t.Run("mismatched associated data fails", func(t *testing.T) {
		fc := newCipher(t)
		ct, _ := fc.Encrypt([]byte("secret"), []byte("record-id:abc123"))
		if _, err := fc.Decrypt(ct, []byte("record-id:DIFFERENT")); err == nil {
			t.Error("want error when associated data differs, got nil")
		}
	})

	t.Run("omitting associated data used at encrypt fails", func(t *testing.T) {
		fc := newCipher(t)
		ct, _ := fc.Encrypt([]byte("secret"), []byte("context"))
		if _, err := fc.Decrypt(ct, nil); err == nil {
			t.Error("want error when associated data omitted but was used at encrypt, got nil")
		}
	})
}
