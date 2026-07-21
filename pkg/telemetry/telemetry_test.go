package telemetry_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.opentelemetry.io/otel"

	"github.com/sousair/gocore/pkg/telemetry"
)

// writeSelfSignedCertPEM writes a throwaway self-signed cert to path so tests
// can force otlp*http.New's env-driven TLS config loader
// (OTEL_EXPORTER_OTLP_CERTIFICATE) to succeed in parsing a root CA — the
// prerequisite for deterministically hitting the "insecure endpoint + TLS
// client config" construction error the exporters return synchronously (no
// network round-trip involved).
func writeSelfSignedCertPEM(t *testing.T, path string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "telemetry-test-ca"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		IsCA:         true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestInit(t *testing.T) {
	t.Run("given no otlp endpoint/when Init and log with span/then json record has service fields and trace_id", func(t *testing.T) {
		t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
		t.Setenv("LOG_LEVEL", "debug")
		buf := &bytes.Buffer{}
		shutdown, err := telemetry.Init(context.Background(), telemetry.Config{
			ServiceName: "testsvc", ServiceVersion: "v0", Writer: buf,
		})
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = shutdown(context.Background()) }()

		tracer := otel.Tracer("t")
		ctx, span := tracer.Start(context.Background(), "op")
		slog.DebugContext(ctx, "hello.world")
		span.End()

		var m map[string]any
		if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
			t.Fatalf("not json: %q", buf.String())
		}
		if m["service_name"] != "testsvc" || m["trace_id"] == nil || m["msg"] != "hello.world" {
			t.Fatalf("record incomplete: %v", m)
		}
	})
	t.Run("given empty service name/when Init/then error", func(t *testing.T) {
		if _, err := telemetry.Init(context.Background(), telemetry.Config{}); err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("given log exporter construction forced to fail/when Init/then error and global tracer provider left untouched", func(t *testing.T) {
		// Forces otlploghttp.New to fail deterministically (no network
		// involved): OTEL_EXPORTER_OTLP_LOGS_ENDPOINT's http:// scheme
		// marks the logs signal insecure, while the generic
		// OTEL_EXPORTER_OTLP_CERTIFICATE supplies a TLS root CA — otlp*http
		// rejects "insecure endpoint + TLS client config" synchronously in
		// New. The trace signal keeps the secure generic
		// OTEL_EXPORTER_OTLP_ENDPOINT, so otlptracehttp.New (and tracer
		// provider construction) succeeds first, exercising the exact
		// ordering Init relies on: log exporter fails after the tracer
		// provider already exists but before any global is mutated.
		certPath := filepath.Join(t.TempDir(), "ca.pem")
		writeSelfSignedCertPEM(t, certPath)
		t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "https://localhost:4318")
		t.Setenv("OTEL_EXPORTER_OTLP_CERTIFICATE", certPath)
		t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "http://localhost:4318")

		before := otel.GetTracerProvider()

		shutdown, err := telemetry.Init(context.Background(), telemetry.Config{ServiceName: "testsvc"})
		if err == nil {
			t.Fatal("want error from forced log-exporter construction failure")
		}
		if shutdown != nil {
			t.Fatal("want nil shutdown on error")
		}
		if got := otel.GetTracerProvider(); got != before {
			t.Fatalf("global tracer provider mutated despite Init error: got %v, want unchanged %v", got, before)
		}
	})
	t.Run("given otlp endpoint set/when Init/then global meter provider wired and shutdown succeeds", func(t *testing.T) {
		// A stub collector (any 200) lets tracer/logger/meter provider
		// shutdown all flush successfully, proving the meter provider's
		// Shutdown is really in the chain rather than asserting on a
		// network-error string.
		collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer collector.Close()
		t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", collector.URL)
		buf := &bytes.Buffer{}

		before := otel.GetMeterProvider()
		shutdown, err := telemetry.Init(context.Background(), telemetry.Config{ServiceName: "testsvc", Writer: buf})
		if err != nil {
			t.Fatal(err)
		}

		if got := otel.GetMeterProvider(); got == before {
			t.Fatal("want global meter provider wired when otlp endpoint set")
		}
		if err := shutdown(context.Background()); err != nil {
			t.Fatalf("want shutdown to succeed (drains meter provider too), got %v", err)
		}
	})
	t.Run("given no otlp endpoint/when Init/then global meter provider left as no-op default", func(t *testing.T) {
		t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
		buf := &bytes.Buffer{}
		before := otel.GetMeterProvider()

		shutdown, err := telemetry.Init(context.Background(), telemetry.Config{ServiceName: "testsvc", Writer: buf})
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = shutdown(context.Background()) }()

		if got := otel.GetMeterProvider(); got != before {
			t.Fatalf("want global meter provider unchanged without otlp endpoint, got %v want %v", got, before)
		}
	})
}
