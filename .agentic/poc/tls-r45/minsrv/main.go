// Minimal Go TLS server (stdlib only) for R-45 ③ isolation: same toolchain, self-signed ECDSA cert, trivial handler.
// usage: go run . <addr> <ca-out.pem> [tls12]
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

type counter struct{ n atomic.Int64 }

func (c *counter) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "handshake error") {
		if c.n.Add(1) <= 3 {
			os.Stderr.Write(p)
		}
	}
	return len(p), nil
}

func main() {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "nsx.cloudinfra-sim.test"},
		DNSNames: []string{"nsx.cloudinfra-sim.test"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign}
	der, _ := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	_ = os.WriteFile(os.Args[2], pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)
	cfg := &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}, MinVersion: tls.VersionTLS12}
	if len(os.Args) > 3 && os.Args[3] == "tls12" {
		cfg.MaxVersion = tls.VersionTLS12
	}
	if os.Getenv("CHURN") != "" { // GC 압력: 할당·폐기를 반복하는 고루틴
		for i := 0; i < 8; i++ {
			go func() {
				var keep [][]byte
				for {
					keep = append(keep, make([]byte, 64<<10))
					if len(keep) > 256 {
						keep = keep[:0]
					}
				}
			}()
		}
	}
	c := &counter{}
	srv := &http.Server{Addr: os.Args[1], TLSConfig: cfg, ErrorLog: log.New(c, "", 0),
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, "ok") })}
	go func() {
		for range time.Tick(5 * time.Second) {
			fmt.Fprintf(os.Stderr, "handshake errors so far: %d\n", c.n.Load())
		}
	}()
	log.Fatal(srv.ListenAndServeTLS("", ""))
}
