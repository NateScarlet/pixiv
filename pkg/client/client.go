package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/tidwall/gjson"
)

// Client to send request to pixiv server.
//
// 零值是安全的：它是一个不做任何特殊处理的标准 HTTP 客户端。
// 要得到本库的默认行为（默认传输、默认 User-Agent、环境变量播种的凭据），用 [New]。
// 一个 Client 可被多个 goroutine 并发使用，也可被值拷贝。
type Client struct {
	// 服务地址，由 [WithServerURL] 设置，在 [New] 装配期解析校验。
	serverURL string
	http.Client
}

// EndpointURL returns url for server endpint.
func (c Client) EndpointURL(path string, values *url.Values) *url.URL {
	s := c.serverURL
	if s == "" {
		// 零值客户端按约定使用默认服务地址。
		s = defaultServerURL
	}
	u, err := url.Parse(s)
	if err != nil {
		// 服务地址已在 [New] 装配期校验，运行时到不了这里。
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

// Default 客户端，与 [New] 走同一装配路径。
var Default = New()
