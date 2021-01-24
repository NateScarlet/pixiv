package client

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"
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
				VerifyPeerCertificate: func(certificates [][]byte, _ [][]*x509.Certificate) (err error) {
					certs := make([]*x509.Certificate, len(certificates))
					for i, asn1Data := range certificates {
						cert, err := x509.ParseCertificate(asn1Data)
						if err != nil {
							return err
						}
						certs[i] = cert
					}
					opts := x509.VerifyOptions{
						DNSName:       host,
						Intermediates: x509.NewCertPool(),
					}
					for _, cert := range certs[1:] {
						opts.Intermediates.AddCert(cert)
					}
					cert := certs[0]
					_, err = cert.Verify(opts)
					if err != nil {
						return
					}

					if time.Now().After(cert.NotAfter) {
						return errors.New("pixiv: client: certification is expired")
					}
					if err = cert.VerifyHostname(host); err != nil {
						return
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
	"i.pximg.net":   {},
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
// must set before other settings, because it use a new http.Transport for blocked host
func (c *Client) BypassSNIBlocking() {
	c.Transport = &BypassSNIBlockingTransport{wrapped: c.Transport}
}
