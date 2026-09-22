package dns

import (
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/tidwall/gjson"
)

type DOHResolver interface {
	Resolver
	URL() string
}

// DoHWireFormat 选择查询请求的编码方式。
type DoHWireFormat int

const (
	// DoHWireFormatMessage 是 RFC 8484 的二进制报文接口，以
	// application/dns-message 承载 DNS 报文本身。它是零值，也是默认方式：
	// RFC 8484 要求实现必须支持这种接口，符合标准的服务端都能用。
	DoHWireFormatMessage DoHWireFormat = iota
	// DoHWireFormatJSON 是 RFC 8484 附录中的 JSON 接口（Google 公共 DNS 的
	// JSON API），以 name 查询参数与 Accept: application/dns-json 表达。
	// 它不在标准强制要求的范围内，因此不是默认方式；需要时在端点 URL 上
	// 以 #type=json 显式声明。
	DoHWireFormatJSON
)

// typeFragmentKey 是端点 URL 的 fragment 中用于声明编码方式的键，
// 形如 https://example.com/dns-query#type=json。
const typeFragmentKey = "type"

// parseEndpoint 解析端点 URL，拆出查询地址与编码方式。
//
// 编码方式以 fragment 声明（#type=json）；不认识的内容快速失败而不是忽略
// ——写错的值若被静默忽略，会得到一个「端点拒绝查询」的错误，
// 掩盖 URL 写错这个真实原因。
//
// 返回的地址不含 fragment：fragment 是给客户端的声明，不应发往服务端。
func parseEndpoint(raw string) (string, DoHWireFormat, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", 0, fmt.Errorf("pixiv: dns: DoH 端点地址 %q 无法解析: %w", raw, err)
	}
	// 缺主机名的地址（例如漏写 scheme 的 doh.example/dns-query）不是可用端点，
	// 在这里报错而不是留到查询时以「不支持的协议」呈现。
	if u.Host == "" {
		return "", 0, fmt.Errorf("pixiv: dns: DoH 端点地址 %q 缺少主机名: 完整形式形如 https://doh.example/dns-query", raw)
	}
	format := DoHWireFormatMessage
	if rawFragment := u.Fragment; rawFragment != "" {
		f, err := parseTypeFragment(rawFragment)
		if err != nil {
			return "", 0, fmt.Errorf("pixiv: dns: DoH 端点地址 %q: %w", raw, err)
		}
		format = f
		u.Fragment = ""
	}
	return u.String(), format, nil
}

// parseTypeFragment 解析 fragment 中声明的编码方式。
func parseTypeFragment(fragment string) (DoHWireFormat, error) {
	values, err := url.ParseQuery(fragment)
	if err != nil {
		return 0, fmt.Errorf("fragment %q 无法解析: %w", fragment, err)
	}
	raw := values.Get(typeFragmentKey)
	if raw == "" {
		return 0, fmt.Errorf("fragment %q 不含 %s 参数: 可用写法为 #%s=json 或 #%s=message",
			fragment, typeFragmentKey, typeFragmentKey, typeFragmentKey)
	}
	switch strings.ToLower(raw) {
	case "message", "dns-message":
		return DoHWireFormatMessage, nil
	case "json":
		return DoHWireFormatJSON, nil
	default:
		return 0, fmt.Errorf("%s 的值 %q 无效: 可用值为 json、message", typeFragmentKey, raw)
	}
}

type dohResolver struct {
	url string
	// client 发出查询请求；nil 时使用 http.DefaultClient（遵循进程代理环境变量）。
	client *http.Client
	// format 是查询请求的编码方式，由端点 URL 的 fragment 决定（默认二进制）。
	format DoHWireFormat
	// ownProxyHosts 是自身访问链路依赖的地址（经自身查询的代理等），按系统
	// 解析处理，见 Resolve 的递归说明。
	ownProxyHosts map[string]struct{}
}

func (r *dohResolver) httpClient() *http.Client {
	if r.client == nil {
		return http.DefaultClient
	}
	return r.client
}

// Resolve implements DNSResolver
//
// 递归说明：DoH 查询自身的出网可能依赖代理（查询经 http.DefaultClient，
// 遵循 HTTPS_PROXY），而传输层的解析接缝会把代理地址也交给解析器。
// 若对这些名字再用 DoH 解析，就构成「解析代理 → 需经代理 → 代理地址待解析」
// 的自举依赖。因此解析自身访问链路依赖的代理主机名时直接走系统解析，
// 不发出 DoH 查询。
func (r *dohResolver) Resolve(ctx context.Context, name string) (ip []net.IP, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("dohResolver{'%s'}.Resolve('%s'): %w", r.url, name, err)
		}
	}()

	if _, own := r.ownProxyHosts[strings.ToLower(name)]; own {
		addrs, err := net.DefaultResolver.LookupIPAddr(ctx, name)
		if err != nil {
			return nil, err
		}
		ips := make([]net.IP, 0, len(addrs))
		for _, a := range addrs {
			ips = append(ips, a.IP)
		}
		return ips, nil
	}

	req, err := r.newRequest(ctx, name)
	if err != nil {
		return
	}

	resp, err := r.httpClient().Do(req)
	if err != nil {
		return
	}
	if resp.StatusCode != http.StatusOK {
		// 带上状态码文本：非 2xx 说明端点可达但拒绝了这次查询，
		// 与「连不上」是完全不同的故障，错误信息要能让人区分。
		// 用可判别的错误类型承载状态码，供诊断工具按失败性质归因。
		err = &StatusError{
			StatusCode: resp.StatusCode,
			Status:     http.StatusText(resp.StatusCode),
		}
		return
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}
	return r.parseResponse(data)
}

// newRequest 按解析器的编码方式构造查询请求。
func (r *dohResolver) newRequest(ctx context.Context, name string) (*http.Request, error) {
	if r.format == DoHWireFormatMessage {
		return r.newMessageRequest(ctx, name)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", r.url, nil)
	if err != nil {
		return nil, err
	}
	var q = req.URL.Query()
	q.Set("name", name)
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Accept", "application/dns-json")
	return req, nil
}

// newMessageRequest 构造 RFC 8484 的二进制报文请求。
//
// 用 GET + dns=<base64url> 而不是 POST：GET 可被 HTTP 缓存与代理按 URL 复用，
// 与 JSON 接口的语义更接近，且 dnscrypt-proxy 等实现两种都支持。
func (r *dohResolver) newMessageRequest(ctx context.Context, name string) (*http.Request, error) {
	// 事务 ID 只用于校验响应与请求对应，用固定值即可（RFC 8484 允许）。
	const messageID uint16 = 0
	query, err := buildQueryA(name, messageID)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", r.url, nil)
	if err != nil {
		return nil, err
	}
	var q = req.URL.Query()
	// base64url 不带填充（RFC 8484 4.1）。
	q.Set("dns", base64.RawURLEncoding.EncodeToString(query))
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Accept", "application/dns-message")
	return req, nil
}

// parseResponse 按解析器的编码方式解读响应体。
func (r *dohResolver) parseResponse(data []byte) ([]net.IP, error) {
	if r.format == DoHWireFormatMessage {
		return parseMessageResponse(data)
	}
	return parseJSONResponse(data), nil
}

// StatusError 表示 DoH 端点返回了非成功状态码。
//
// 它与网络错误有本质区别：端点可达且接受了连接，只是拒绝了这次查询。
// 诊断工具据此把「端点不可达」与「端点拒绝查询」区分开——两者的
// 排查方向不同（前者查网络，后者查端点的协议支持）。
type StatusError struct {
	// StatusCode 是端点返回的状态码。
	StatusCode int
	// Status 是状态码的文本，如 "Bad Request"。
	Status string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("status %d %s", e.StatusCode, e.Status)
}

// parseJSONResponse 从 JSON 接口的应答中取出 A 记录地址。
func parseJSONResponse(data []byte) []net.IP {
	var ip []net.IP
	var jsonData = gjson.ParseBytes(data)
	jsonData.Get("Answer.#(type==1)#.data").ForEach(func(key, value gjson.Result) bool {
		ip = append(ip, net.ParseIP(value.String()))
		return true
	})
	return ip
}

// parseMessageResponse 从二进制报文应答中取出 A 记录地址。
func parseMessageResponse(data []byte) ([]net.IP, error) {
	// 构造请求时使用的事务 ID 固定，这里以同一常量校验。
	const messageID uint16 = 0
	addrs, err := parseARecords(data, messageID)
	if err != nil {
		return nil, err
	}
	ip := make([]net.IP, 0, len(addrs))
	for _, addr := range addrs {
		ip = append(ip, net.ParseIP(addr))
	}
	return ip, nil
}

// URL implements DOHResolver
func (r *dohResolver) URL() string {
	return r.url
}

// ResolverOption 调整 DoH 解析器的构造。
type ResolverOption func(*dohResolver)

// WithHTTPClient 指定发出查询请求的 HTTP client。
//
// 未指定时使用 http.DefaultClient（遵循进程代理环境变量 HTTPS_PROXY 等）。
// 需要受控代理行为或自定义 TLS 信任的调用者用本选项注入自己的 client；
// 注入的 client 零值字段按标准库默认处理。
func WithHTTPClient(c *http.Client) ResolverOption {
	return func(r *dohResolver) { r.client = c }
}

// NewDOHResolver 构造 DoH 解析器，查询经 http.DefaultClient 发出。
//
// endpoint 是 DoH 端点地址，编码方式由 URL 的 fragment 声明：
// https://example.com/dns-query 用默认的二进制报文方式（RFC 8484 强制要求
// 实现支持，符合标准的服务端都能用）；需要 JSON 接口时写成
// https://example.com/dns-query#type=json。
//
// 地址无法解析或 fragment 声明非法时 panic：解析器在包级变量初始化时构造，
// 无法返回错误，而带着一个不会生效的地址继续运行只会让失败推迟到首次解析。
func NewDOHResolver(endpoint string, opts ...ResolverOption) DOHResolver {
	return newDOHResolver(endpoint, opts...)
}

// NewDOHResolverWithClient 用指定的 HTTP client 构造 DoH 解析器。
//
// 等价于 NewDOHResolver(endpoint, WithHTTPClient(client))，保留本函数是为了
// 不破坏既有调用方。
func NewDOHResolverWithClient(endpoint string, client *http.Client) DOHResolver {
	return newDOHResolver(endpoint, WithHTTPClient(client))
}

// newDOHResolver 是所有构造函数的共同装配点。
//
// 各构造函数都会把进程代理环境变量指向的主机名登记为自身访问链路的依赖，
// 见 [dohResolver.Resolve] 的递归说明。
func newDOHResolver(endpoint string, opts ...ResolverOption) *dohResolver {
	queryURL, format, err := parseEndpoint(endpoint)
	if err != nil {
		panic(err.Error())
	}
	r := &dohResolver{
		url:           queryURL,
		format:        format,
		ownProxyHosts: ownProxyHostsFromEnv(),
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(r)
	}
	return r
}

// ownProxyHostsFromEnv 取进程代理环境变量（与 http.ProxyFromEnvironment
// 的取值集合一致）中各代理地址的主机名部分。代理是 DoH 查询自身的出网
// 途径，对这些名字的解析是解析器的自举依赖。
func ownProxyHostsFromEnv() map[string]struct{} {
	var hosts map[string]struct{}
	for _, key := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"} {
		raw := os.Getenv(key)
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" {
			continue
		}
		if hosts == nil {
			hosts = make(map[string]struct{})
		}
		hosts[strings.ToLower(u.Hostname())] = struct{}{}
	}
	return hosts
}
