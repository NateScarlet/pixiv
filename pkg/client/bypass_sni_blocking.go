package client

import (
	"crypto/tls"
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
		return t.wrapped.RoundTrip(req)
	}

	ip, err := resolveHostname(req.URL.Host)
	req.Host = req.URL.Host
	req.URL.Host = ip

	// XXX: insecure transpot
	var it = new(http.Transport)
	it.TLSClientConfig = new(tls.Config)
	it.TLSClientConfig.InsecureSkipVerify = true
	return it.RoundTrip(req)

}

// BypassSNIBlocking wrap current transport with bypass sni blocking support.
func (c *Client) BypassSNIBlocking() {
	c.Transport = BypassSNIBlockingTransport{c.Transport}
}
