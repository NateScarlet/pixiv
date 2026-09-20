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

// setDefaultHeader 为所有请求设置默认请求头，已有值时不覆盖调用者的设置。
//
// 它包在传输之上，因此对经过路由的请求同样生效——路由只改变用哪个底层传输
// 建立连接，不改变请求本身，因此不再有「必须最先调用」的顺序要求。
func (c *Client) setDefaultHeader(key, value string) {
	c.SetRequestOptions(func(req *http.Request) {
		if len(req.Header[key]) > 0 {
			return
		}
		req.Header.Set(key, value)
	})

}
