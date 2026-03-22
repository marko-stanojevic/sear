package agent

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/marko-stanojevic/kompakt/internal/common"
)

// selfSignedCACertPEM generates a minimal self-signed CA certificate in PEM format.
func selfSignedCACertPEM(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-ca"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IsCA:         true,
		KeyUsage:     x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := pem.Encode(&buf, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func writeTempFile(t *testing.T, content []byte) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "*.pem")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(content); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	return f.Name()
}

func TestBuildTLSConfig_NeitherSet_ReturnsNil(t *testing.T) {
	cfg, err := buildTLSConfig(&common.AgentConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Fatal("expected nil when neither TLSSkipVerify nor TLSCAFile is set")
	}
}

func TestBuildTLSConfig_SkipVerify(t *testing.T) {
	cfg, err := buildTLSConfig(&common.AgentConfig{TLSSkipVerify: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil TLS config")
	}
	if !cfg.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be true")
	}
}

func TestBuildTLSConfig_CAFile_Valid(t *testing.T) {
	caPath := writeTempFile(t, selfSignedCACertPEM(t))

	cfg, err := buildTLSConfig(&common.AgentConfig{TLSCAFile: caPath})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil TLS config")
	}
	if cfg.RootCAs == nil {
		t.Fatal("RootCAs should be populated")
	}
	if cfg.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should remain false")
	}
}

func TestBuildTLSConfig_CAFile_NotFound(t *testing.T) {
	_, err := buildTLSConfig(&common.AgentConfig{TLSCAFile: "/nonexistent/ca.crt"})
	if err == nil {
		t.Fatal("expected error for missing CA file")
	}
}

func TestBuildTLSConfig_CAFile_InvalidPEM(t *testing.T) {
	caPath := writeTempFile(t, []byte("this is not valid PEM"))

	_, err := buildTLSConfig(&common.AgentConfig{TLSCAFile: caPath})
	if err == nil {
		t.Fatal("expected error for file with no valid PEM blocks")
	}
}

// When both are set, TLSSkipVerify takes precedence (early return in buildTLSConfig).
// The CA file path is never read, so a nonexistent path produces no error.
func TestBuildTLSConfig_SkipVerify_TakesPrecedenceOverCAFile(t *testing.T) {
	cfg, err := buildTLSConfig(&common.AgentConfig{
		TLSSkipVerify: true,
		TLSCAFile:     "/nonexistent/ignored.crt",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be true")
	}
}
