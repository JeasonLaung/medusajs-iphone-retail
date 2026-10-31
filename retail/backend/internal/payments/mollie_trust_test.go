package payments

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestMollieExplicitTrustStillVerifiesCertificateAndHost(t *testing.T) {
	for _, hostname := range []string{"api.mollie.com", "other.example.invalid"} {
		public, private, e := ed25519.GenerateKey(rand.Reader)
		if e != nil {
			t.Fatal(e)
		}
		template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: hostname}, DNSNames: []string{hostname}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
		der, e := x509.CreateCertificate(rand.Reader, template, template, public, private)
		if e != nil {
			t.Fatal(e)
		}
		cert, e := x509.ParseCertificate(der)
		if e != nil {
			t.Fatal(e)
		}
		server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Host != "api.mollie.com" {
				t.Error("fixed provider host changed")
			}
			_ = json.NewEncoder(w).Encode(refundParent())
		}))
		server.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: private}}, MinVersion: tls.VersionTLS12}
		server.StartTLS()
		target, _ := url.Parse(server.URL)
		for _, trust := range []string{"platform", "unrelated", "explicit"} {
			t.Run(hostname+"/"+trust, func(t *testing.T) {
				config := mollieCreateConfig()
				if trust != "platform" {
					config.RootCAs = x509.NewCertPool()
					if trust == "explicit" {
						config.RootCAs.AddCert(cert)
					}
				}
				adapter, e := NewMollieCreateAdapter(config)
				if e != nil {
					t.Fatal(e)
				}
				var transport *http.Transport
				if config.RootCAs == nil {
					if adapter.transport.client.Transport != nil {
						t.Fatal("default platform trust changed")
					}
					transport = http.DefaultTransport.(*http.Transport).Clone()
				} else {
					transport = adapter.transport.client.Transport.(*http.Transport)
					if transport.TLSClientConfig.InsecureSkipVerify || transport.TLSClientConfig.RootCAs == config.RootCAs {
						t.Fatal("mutable or insecure trust configuration")
					}
				}
				// Redirect the socket only. The production request URL, TLS hostname,
				// provider identity checks, credentials and response validation remain real.
				transport.Proxy = nil
				transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
					if address != "api.mollie.com:443" {
						t.Fatal("unexpected provider destination")
					}
					return (&net.Dialer{}).DialContext(ctx, network, target.Host)
				}
				adapter.transport.client.Transport = transport
				defer transport.CloseIdleConnections()
				observed, e := adapter.ObserveSession(context.Background(), refundFixtureRequest().Session, "tr_owned")
				accepted := hostname == "api.mollie.com" && trust == "explicit"
				if accepted && (e != nil || observed.ProviderStatus != "paid") {
					t.Fatal("explicit CA refused valid provider", e)
				}
				if !accepted && e == nil {
					t.Fatal("untrusted certificate or wrong hostname accepted")
				}
			})
		}
		server.Close()
	}
}
