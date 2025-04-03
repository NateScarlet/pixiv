package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/NateScarlet/pixiv/pkg/client/dns"
	"github.com/tidwall/gjson"
)

// Client to send request to pixiv server.
type Client struct {
	ServerURL   string
	DNSResolver dns.Resolver
	http.Client
}

// EndpointURL returns url for server endpint.
func (c Client) EndpointURL(path string, values *url.Values) *url.URL {
	s := c.ServerURL
	if s == "" {
		s = "https://www.pixiv.net"
	}

	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	u.Path = path
	if values != nil {
		u.RawQuery = values.Encode()
	}
	return u
}

// GetWithContext create get request with context and do it.
func (c *Client) GetWithContext(ctx context.Context, url string) (resp *http.Response, err error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

func ParseAPIResponse(r io.Reader) (_ json.RawMessage, err error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return
	}
	if !gjson.ValidBytes(data) {
		err = fmt.Errorf("pixiv: client: invalid json: %q", string(data))
		return
	}
	var res = gjson.ParseBytes(data)
	hasError := res.Get("error").Bool()
	message := res.Get("message").String()
	res = res.Get("body")
	if hasError {
		return data, fmt.Errorf("pixiv: client: api error: %s", message)
	}
	return json.RawMessage(res.Raw), err
}

// Deprecated: use [ParseAPIResponse] instead.
// ParseAPIResult parses error from json api response, and returns body part.
func ParseAPIResult(r io.Reader) (ret gjson.Result, err error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return
	}
	s := string(data)
	if !gjson.Valid(s) {
		err = fmt.Errorf("pixiv: client: invalid json: %s", s)
		return
	}
	ret = gjson.Parse(s)
	hasError := ret.Get("error").Bool()
	message := ret.Get("message").String()
	ret = ret.Get("body")
	if hasError {
		err = fmt.Errorf("pixiv: client: api error: %s", message)
	}
	return
}
