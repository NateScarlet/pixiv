package client

import "net/http"

// RequestOption can mutate request before actual send it.
type RequestOption = func(req *http.Request)

// RequestOptionsTransport allow change request before do it.
type RequestOptionsTransport struct {
	wrapped http.RoundTripper
	options []RequestOption
}

// RoundTrip implements http.RoundTripper
func (t *RequestOptionsTransport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	for _, i := range t.options {
		i(req)
	}
	if t.wrapped == nil {
		return http.DefaultTransport.RoundTrip(req)
	}
	return t.wrapped.RoundTrip(req)
}

// SetRequestOptions for all requests
func (c *Client) SetRequestOptions(options ...RequestOption) {
	c.Transport = &RequestOptionsTransport{
		wrapped: c.Transport,
		options: options,
	}

}

// SetDefaultHeader for all requests
func (c *Client) SetDefaultHeader(key, value string) {
	c.SetRequestOptions(func(req *http.Request) {
		if len(req.Header[key]) > 0 {
			return
		}
		req.Header.Set(key, value)
	})

}
