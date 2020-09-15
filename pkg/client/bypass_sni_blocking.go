package client

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"sync"
)

// BypassSNIBlockingTransport bypass sni bloking when host in BlockedHostnames
type BypassSNIBlockingTransport struct {
	wrapped                http.RoundTripper
	antiSNIDetectTransport http.RoundTripper
	mu                     sync.Mutex
}

func (t *BypassSNIBlockingTransport) ensureWrappedTransport() http.RoundTripper {
	if t.wrapped == nil {
		return http.DefaultTransport
	}
	return t.wrapped
}

func (t *BypassSNIBlockingTransport) ensureAntiSNIDetectTransport() http.RoundTripper {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.antiSNIDetectTransport == nil {
		var v = new(http.Transport)
		v.DialTLS = func(network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ip, err := resolveHostname(host)
			if err != nil {
				return nil, err
			}
			return tls.Dial(network, net.JoinHostPort(ip, port), &tls.Config{
				InsecureSkipVerify: true,
				VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) (err error) {
					certs, err := x509.ParseCertificates(bytes.Join(rawCerts, nil))
					if err != nil {
						return
					}
					// need a valid cert for host
					for _, i := range certs {
						_, err = i.Verify(x509.VerifyOptions{})
						if err != nil {
							// ignore invalid cert
							continue
						}
						err = i.VerifyHostname(host)
						if err == nil {
							break
						}
					}
					return
				},
			})
		}
		t.antiSNIDetectTransport = v
	}
	return t.antiSNIDetectTransport

}

// BlockedHostnames constains hosts that blocked by sni detect.
var BlockedHostnames = map[string]struct{}{
	"www.pixiv.net": {},
}

// RoundTrip implements http.RoundTripper
func (t *BypassSNIBlockingTransport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	if _, ok := BlockedHostnames[req.URL.Host]; !ok {
		// skip not blocked.
		return t.ensureWrappedTransport().RoundTrip(req)
	}
	return t.ensureAntiSNIDetectTransport().RoundTrip(req)
}

// BypassSNIBlocking wrap current transport with bypass sni blocking support.
// SECURITY WARNING: TLS verification will be disabled for blocked site.
func (c *Client) BypassSNIBlocking() {
	c.Transport = &BypassSNIBlockingTransport{wrapped: c.Transport}
}
