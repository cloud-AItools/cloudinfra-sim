// Package certs는 시뮬레이터 루트 CA와 서버 인증서를 데이터 디렉터리에 만들고 유지한다.
// 관련: NFR-0302(HTTPS), NFR-0303(교체), NFR-0313(SAN *.<도메인>), NFR-0315(지문 안정성).
package certs

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	caCertFile     = "ca.pem"
	caKeyFile      = "ca-key.pem"
	serverCertFile = "server.pem"
	serverKeyFile  = "server-key.pem"
)

// Bundle은 서버 TLS 설정에 쓰는 인증서 묶음이다.
type Bundle struct {
	Server tls.Certificate
	CA     *x509.Certificate
	CAPool *x509.CertPool
}

// Thumbprint는 서버 인증서의 SHA-1 지문(vCenter·Compute Manager 등록 관례, 콜론 구분)이다.
func (b *Bundle) Thumbprint() string { return colon(sha1sum(b.Server.Certificate[0])) }

// ThumbprintSHA256은 SHA-256 지문(콜론 구분)이다.
func (b *Bundle) ThumbprintSHA256() string {
	s := sha256.Sum256(b.Server.Certificate[0])
	return colon(s[:])
}

func sha1sum(b []byte) []byte { s := sha1.Sum(b); return s[:] }

func colon(b []byte) string {
	parts := make([]string, len(b))
	for i, x := range b {
		parts[i] = fmt.Sprintf("%02X", x)
	}
	return strings.Join(parts, ":")
}

// LoadOrCreate는 dir의 인증서를 읽고, 없으면 만든다. 기존 인증서의 SAN이 도메인과 맞지 않으면
// 오류를 돌려준다(지문을 조용히 바꾸지 않는다 — NFR-0315). 교체는 Rotate로만 한다.
func LoadOrCreate(dir, domain string) (*Bundle, bool, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, false, err
	}
	b, err := load(dir)
	if err == nil {
		if !covers(b.Server, domain) {
			return nil, false, fmt.Errorf("기존 서버 인증서가 *.%s를 포함하지 않는다. 도메인을 바꿨다면 'cloudinfra-sim cert rotate'로 교체한다", domain)
		}
		return b, false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, false, err
	}
	b, err = create(dir, domain)
	return b, true, err
}

// Rotate는 서버 인증서만 새로 발급한다(CA 유지, NFR-0303). 새 지문을 돌려준다.
func Rotate(dir, domain string) (*Bundle, error) {
	ca, caKey, err := loadCA(dir)
	if err != nil {
		return nil, err
	}
	if err := issueServer(dir, domain, ca, caKey); err != nil {
		return nil, err
	}
	return load(dir)
}

func covers(c tls.Certificate, domain string) bool {
	leaf, err := x509.ParseCertificate(c.Certificate[0])
	if err != nil {
		return false
	}
	return leaf.VerifyHostname("vcenter."+domain) == nil && leaf.VerifyHostname("nsx."+domain) == nil
}

func load(dir string) (*Bundle, error) {
	server, err := tls.LoadX509KeyPair(filepath.Join(dir, serverCertFile), filepath.Join(dir, serverKeyFile))
	if err != nil {
		if _, statErr := os.Stat(filepath.Join(dir, serverCertFile)); errors.Is(statErr, os.ErrNotExist) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	ca, _, err := loadCA(dir)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	return &Bundle{Server: server, CA: ca, CAPool: pool}, nil
}

func loadCA(dir string) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	certPEM, err := os.ReadFile(filepath.Join(dir, caCertFile))
	if err != nil {
		return nil, nil, err
	}
	keyPEM, err := os.ReadFile(filepath.Join(dir, caKeyFile))
	if err != nil {
		return nil, nil, err
	}
	cb, _ := pem.Decode(certPEM)
	kb, _ := pem.Decode(keyPEM)
	if cb == nil || kb == nil {
		return nil, nil, errors.New("CA PEM 형식 오류")
	}
	ca, err := x509.ParseCertificate(cb.Bytes)
	if err != nil {
		return nil, nil, err
	}
	key, err := x509.ParseECPrivateKey(kb.Bytes)
	return ca, key, err
}

func create(dir, domain string) (*Bundle, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	tpl := &x509.Certificate{
		SerialNumber:          serial(),
		Subject:               pkix.Name{CommonName: "cloudinfra-sim Root CA", Organization: []string{"cloudinfra-sim (test only)"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		BasicConstraintsValid: true,
		MaxPathLenZero:        true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	ca, _ := x509.ParseCertificate(der)
	if err := writePEM(filepath.Join(dir, caCertFile), "CERTIFICATE", der, 0o644); err != nil {
		return nil, err
	}
	kb, _ := x509.MarshalECPrivateKey(key)
	if err := writePEM(filepath.Join(dir, caKeyFile), "EC PRIVATE KEY", kb, 0o600); err != nil {
		return nil, err
	}
	if err := issueServer(dir, domain, ca, key); err != nil {
		return nil, err
	}
	return load(dir)
}

func issueServer(dir, domain string, ca *x509.Certificate, caKey *ecdsa.PrivateKey) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	tpl := &x509.Certificate{
		SerialNumber: serial(),
		Subject:      pkix.Name{CommonName: "*." + domain},
		DNSNames:     []string{"*." + domain},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(2, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, ca, &key.PublicKey, caKey)
	if err != nil {
		return err
	}
	chain := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	chain = append(chain, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw})...)
	if err := os.WriteFile(filepath.Join(dir, serverCertFile), chain, 0o644); err != nil {
		return err
	}
	kb, _ := x509.MarshalECPrivateKey(key)
	return writePEM(filepath.Join(dir, serverKeyFile), "EC PRIVATE KEY", kb, 0o600)
}

func writePEM(path, typ string, der []byte, mode os.FileMode) error {
	return os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der}), mode)
}

func serial() *big.Int {
	n, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	return n
}
