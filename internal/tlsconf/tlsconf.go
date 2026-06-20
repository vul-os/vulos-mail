// Package tlsconf provides helpers for building *tls.Config values used by the
// SMTP/IMAP servers (STARTTLS and implicit TLS) and the HTTPS API.
//
// It supports three modes:
//
//   - FromFiles loads a PEM certificate/key pair from disk.
//   - SelfSigned generates an in-memory self-signed certificate, for dev/test.
//   - Config picks between the two (or returns nil, nil meaning "TLS disabled")
//     based on which arguments are supplied.
//
// Only the standard library is used.
package tlsconf

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// certValidity is how long a generated self-signed certificate remains valid.
const certValidity = 365 * 24 * time.Hour

// FromFiles loads the PEM-encoded certificate and key pair at certFile and
// keyFile and returns a *tls.Config that presents that certificate, with a
// minimum protocol version of TLS 1.2.
func FromFiles(certFile, keyFile string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("tlsconf: load key pair: %w", err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// SelfSigned generates an in-memory self-signed ECDSA (P-256) certificate valid
// for roughly one year for the given hosts. Each host may be a DNS name or an IP
// address; IP addresses are added as IP SANs and everything else as DNS SANs. If
// no hosts are given, "localhost" is used. It returns a ready-to-use *tls.Config
// with a minimum protocol version of TLS 1.2.
//
// SelfSigned is intended for development and testing only.
func SelfSigned(hosts ...string) (*tls.Config, error) {
	cert, err := selfSignedCert(hosts...)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// Config selects a TLS configuration based on the supplied arguments:
//
//   - if both certFile and keyFile are non-empty, it loads them via FromFiles;
//   - otherwise, if any selfSignedHosts are given, it generates one via SelfSigned;
//   - otherwise it returns (nil, nil), meaning TLS is disabled.
func Config(certFile, keyFile string, selfSignedHosts ...string) (*tls.Config, error) {
	if certFile != "" && keyFile != "" {
		return FromFiles(certFile, keyFile)
	}
	if len(selfSignedHosts) > 0 {
		return SelfSigned(selfSignedHosts...)
	}
	return nil, nil
}

// WritePEM generates a self-signed certificate/key pair for "localhost" and
// 127.0.0.1 and writes them as cert.pem and key.pem into dir, returning their
// paths. It is a convenience for tooling that needs cert files on disk.
func WritePEM(dir string) (certFile, keyFile string, err error) {
	certPEM, keyPEM, err := selfSignedPEM("localhost", "127.0.0.1")
	if err != nil {
		return "", "", err
	}
	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, certPEM, 0o644); err != nil {
		return "", "", fmt.Errorf("tlsconf: write cert: %w", err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		return "", "", fmt.Errorf("tlsconf: write key: %w", err)
	}
	return certFile, keyFile, nil
}

// selfSignedPEM generates a self-signed certificate for the given hosts and
// returns the PEM-encoded certificate and private key.
func selfSignedPEM(hosts ...string) (certPEM, keyPEM []byte, err error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("tlsconf: generate key: %w", err)
	}

	template, err := certTemplate(hosts...)
	if err != nil {
		return nil, nil, err
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, fmt.Errorf("tlsconf: create certificate: %w", err)
	}

	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, nil, fmt.Errorf("tlsconf: marshal key: %w", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}

// selfSignedCert generates a self-signed certificate for the given hosts and
// returns it as a tls.Certificate ready to be placed in a tls.Config.
func selfSignedCert(hosts ...string) (tls.Certificate, error) {
	certPEM, keyPEM, err := selfSignedPEM(hosts...)
	if err != nil {
		return tls.Certificate{}, err
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("tlsconf: assemble key pair: %w", err)
	}
	return cert, nil
}

// certTemplate builds an x509 certificate template with sensible defaults and
// the supplied hosts as subject alternative names.
func certTemplate(hosts ...string) (*x509.Certificate, error) {
	if len(hosts) == 0 {
		hosts = []string{"localhost"}
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("tlsconf: generate serial: %w", err)
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{Organization: []string{"vulos-mail self-signed"}},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(certValidity),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	for _, h := range hosts {
		if h == "" {
			return nil, errors.New("tlsconf: empty host name")
		}
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}

	return template, nil
}
