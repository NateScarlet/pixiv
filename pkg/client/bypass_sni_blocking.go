package client

import (
	"crypto/tls"
	"net"
	"net/http"
)

// BypassSNIBlockingTransport bypass sni bloking when host in BlockedHostnames
type BypassSNIBlockingTransport struct {
	wrapped http.RoundTripper
}

// BlockedHostnames constains hosts that blocked by sni detect.
var BlockedHostnames = map[string]struct{}{
	"www.pixiv.net": {},
}

// RoundTrip implements http.RoundTripper
func (t BypassSNIBlockingTransport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	if _, ok := BlockedHostnames[req.URL.Host]; !ok {
		// skip not blocked.
		if t.wrapped == nil {
			return http.DefaultTransport.RoundTrip(req)
		}
		return t.wrapped.RoundTrip(req)
	}

	// XXX: insecure transpot
	var it = new(http.Transport)
	it.DialTLS = func(network, addr string) (net.Conn, error) {
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
		})
	}
	return it.RoundTrip(req)

}

// BypassSNIBlocking wrap current transport with bypass sni blocking support.
// SECURITY WARNING: TLS verification will be disabled for blocked site.
func (c *Client) BypassSNIBlocking() {
	c.Transport = BypassSNIBlockingTransport{c.Transport}
}
