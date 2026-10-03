package client

import (
	"bytes"
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

// ErrAPIRejected 表示服务端以失败状态码拒绝了这次 API 请求（例如 403、404、429、503），
// 因此响应体不是本库要解析的 JSON。调用者可用它分辨「被拒绝」与「响应格式不对」，
// 前者通常值得退避重试或提示重新登录。
//
// 这是携带本次请求数据的结构化错误，用 errors.As 取回，不是 errors.Is：
//
//	var rej *ErrAPIRejected
//	if errors.As(err, &rej) {
//		log.Println(rej.Response.StatusCode)
//	}
//
// [ParseAPIResponseV2] 对非 2xx 状态返回本类型的指针，调用者再包一层错误也能取回。
//
// [CheckAPIResponse] 同样返回本类型。
//
// Response 是唯一的状态来源：状态码、状态行、请求 URI、响应头都在其中。
// 其 Body 从不被本库的拒绝路径读取；[ParseAPIResponseV2] 报错时已关闭它，
// 直接使用 [CheckAPIResponse] 的调用方要自己负责关闭。
type ErrAPIRejected struct {
	// Response 是被拒绝的响应，仅供读取元数据。
	Response *http.Response
}

// Error 实现 [error]。文本只含状态行，不带响应体——被边缘节点拒绝时响应体常是
// 整页 HTML，对调用者没有价值。
func (e *ErrAPIRejected) Error() string {
	if e.Response == nil {
		// 零值未携带响应，没有状态可报。
		return "pixiv: client: api 请求被拒绝"
	}
	return "pixiv: client: api 请求被拒绝: " + e.Response.Status
}

// CheckAPIResponse 校验响应状态，成功返回 nil，非 2xx 返回 [ErrAPIRejected]。
//
// 状态码只有从响应本身才读得到，因此校验必须发生在解析之前：[ParseAPIResponseV2]
// 为有 {error, body} 信封的端点两步一起做完，而没有信封的旧式端点
// （ranking.php 的 contents 在顶层）用本函数校验后自己读响应体。
// 不校验的后果是把边缘节点的整页 HTML 当成数据返回给调用方，err == nil，
// 调用方无从按状态码退避重试，也分不清「被拒绝」与「响应格式不对」。
//
// 本函数只判状态：不读也不关响应体，关闭由调用方负责（[ParseAPIResponseV2] 已代为关闭）。
func CheckAPIResponse(resp *http.Response) error {
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &ErrAPIRejected{Response: resp}
	}
	return nil
}

// ParseAPIResponseV2 校验响应状态并解析 API 响应体，返回信封中 body 部分的原始 JSON。
//
// 它是 [ParseAPIResponse] 的替代：状态码只有从响应本身才读得到，
// 因此由本函数一并校验，调用者不必（也无法）在别处补这一步。
//
// 成功状态（2xx）之外的响应以 [ErrAPIRejected] 报错，错误里带上被拒响应
// （状态码在其中，用 errors.As 取回），但不读取响应体——被边缘节点拒绝时响应体常是
// 整页 HTML，对调用者没有价值。失败路径下响应体已由本函数关闭。
//
// 成功状态下仍需是可解析的 JSON 信封：信封里 error 为真时报出其中的 message，
// error 直接是错误信息字符串时按该字符串报错，否则返回 body 字段的原始 JSON。
// 204 这类无正文的成功响应按空值处理。
//
// 响应体不是 {error, body} 信封的旧式端点（ranking.php）不能走本函数，
// 那种响应要自己配 [CheckAPIResponse] 加自己的解析。
func ParseAPIResponseV2(resp *http.Response) (_ json.RawMessage, err error) {
	defer resp.Body.Close()
	if err := CheckAPIResponse(resp); err != nil {
		// 状态行已足以说明失败原因，不读取响应体。
		return nil, err
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
	hasError, message := apiError(res)
	res = res.Get("body")
	if hasError {
		return nil, fmt.Errorf("pixiv: client: api error: %s", message)
	}
	return json.RawMessage(res.Raw), nil
}

// apiError 判断信封是否表示失败，并给出错误消息。
//
// 服务端有两种失败形态：error 为真、消息在 message 字段；
// 以及 error 直接是错误信息字符串（例如 {"error":"不在排行榜统计范围内"}），
// 后者没有 message/body，必须按字符串内容判错，否则会被当成成功返回空 body。
func apiError(res gjson.Result) (hasError bool, message string) {
	errField := res.Get("error")
	isStringError := errField.Type == gjson.String && errField.Str != ""
	message = res.Get("message").String()
	if isStringError && message == "" {
		message = errField.Str
	}
	return errField.Bool() || isStringError, message
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
	hasError, message := apiError(ret)
	ret = ret.Get("body")
	if hasError {
		err = fmt.Errorf("pixiv: client: api error: %s", message)
	}
	return
}

// Default 客户端，与 [New] 走同一装配路径。
var Default = New()
