package agent

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"github.com/marko-stanojevic/kompakt/internal/common"
)

// buildTLSConfig constructs a *tls.Config from the agent configuration.
// Returns nil when neither TLSCAFile nor TLSSkipVerify is set, meaning the
// caller should use the system default TLS configuration.
func buildTLSConfig(cfg *common.AgentConfig) (*tls.Config, error) {
	if !cfg.TLSSkipVerify && cfg.TLSCAFile == "" {
		return nil, nil
	}
	if cfg.TLSSkipVerify {
		return &tls.Config{InsecureSkipVerify: true}, nil //nolint:gosec // intentional, user-configured
	}
	pem, err := os.ReadFile(cfg.TLSCAFile)
	if err != nil {
		return nil, fmt.Errorf("reading tls_ca_file: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("tls_ca_file %q contains no valid PEM certificates", cfg.TLSCAFile)
	}
	return &tls.Config{RootCAs: pool}, nil
}
