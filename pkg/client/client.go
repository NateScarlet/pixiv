package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

// ErrAPIRejected 表示服务端以失败状态码拒绝了这次 API 请求（例如 403、429、503），
// 因此响应体不是本库要解析的 JSON。调用者可用 [errors.Is] 分辨「被拒绝」与
// 「响应格式不对」，前者通常值得退避重试或提示重新登录。
var ErrAPIRejected = errors.New("pixiv: client: api 请求被拒绝")

// ParseAPIResponseV2 校验响应状态并解析 API 响应体，返回信封中 body 部分的原始 JSON。
//
// 它是 [ParseAPIResponse] 的替代：状态码只有从响应本身才读得到，
// 因此由本函数一并校验，调用者不必（也无法）在别处补这一步。
//
// 成功状态（2xx）之外的响应以 [ErrAPIRejected] 报错，错误里带上状态行，
// 但不带响应体——被边缘节点拒绝时响应体常是整页 HTML，对调用者没有价值。
// 失败路径下响应体已由本函数读完并关闭。
//
// 成功状态下仍需是可解析的 JSON 信封：信封里 error 为真时报出其中的 message，
// 否则返回 body 字段的原始 JSON。204 这类无正文的成功响应按空值处理。
func ParseAPIResponseV2(resp *http.Response) (_ json.RawMessage, err error) {
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		// 状态行已足以说明失败原因，不读取响应体。
		return nil, fmt.Errorf("%w: %s", ErrAPIRejected, resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		// 204 与空 200 已由状态码判定为成功，只是其中没有需要解析的内容。
		return nil, nil
	}
	if !gjson.ValidBytes(data) {
		return nil, fmt.Errorf("pixiv: client: invalid json: %q", string(data))
	}
	var res = gjson.ParseBytes(data)
	hasError := res.Get("error").Bool()
	message := res.Get("message").String()
	res = res.Get("body")
	if hasError {
		return nil, fmt.Errorf("pixiv: client: api error: %s", message)
	}
	return json.RawMessage(res.Raw), nil
}

// Deprecated: use [ParseAPIResponseV2] instead.
// ParseAPIResult parses error from json api response, and returns body part.
//
// 它同样不接收响应本身，因此也无法校验状态码。
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
