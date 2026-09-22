package dns

import (
	"context"
	"fmt"
	"net"
	"net/url"
)

// 端点地址支持的 scheme。PIXIV_DNS_QUERY_URL 的取值语义全部由本包定义，
// 因此 scheme 分派也在这里：调用者只给出一个字符串，不需要理解
// #type=json 这类 fragment 语法。
const (
	schemeHTTP  = "http"
	schemeHTTPS = "https"
	schemeDNS   = "dns"
)

// endpointHint 列出端点地址的可用写法，用于报错信息。
//
// 写法非法时静默回落会得到与实际配置无关的解析路径（例如漏写 // 时
// 以为指定了服务器、实际走了系统解析），因此这些错误都在构造期以 panic
// 呈现，并给出正确写法。
const endpointHint = "可用写法为 http(s)://…（DoH）、dns://<ip>[:port]（传统 DNS）、dns:（系统解析）"

// defaultDNSPort 是 dns:// 未声明端口时使用的端口。
const defaultDNSPort = "53"

// NewResolver 按端点地址的 scheme 构造解析器。
//
// 可用写法：
//   - http(s)://… —— DoH，查询方式由 URL 的 fragment 声明（见 NewDOHResolver）。
//   - dns://<ip>[:port] —— 明文 DNS，把查询发往该服务器，端口缺省 53。
//   - dns: —— 系统解析，与显式传入 WithDNSResolver(nil) 同义。
//
// opts 只对 DoH 端点有意义（例如注入发出查询的 HTTP client）；明文 DNS 与
// 系统解析没有对应概念，传给它们会 panic。
//
// 地址无法解析或写法非法时 panic：解析器在包级变量初始化时构造，无法返回
// 错误，而带着一个不会生效的地址继续运行只会让失败推迟到首次解析。
func NewResolver(endpoint string, opts ...ResolverOption) Resolver {
	scheme, u := parseScheme(endpoint)
	if scheme == schemeDNS {
		if len(opts) > 0 {
			panic(fmt.Sprintf("pixiv: dns: 端点地址 %q: dns 方式不接受解析器选项: WithHTTPClient 只对 DoH 端点有意义", endpoint))
		}
		return newDNSResolver(endpoint, u)
	}
	// http 同样是既有的可用取值：本地 DoH 服务（如 dnscrypt-proxy 的
	// 明文监听）常以 http 提供，DoH 解析器本身不校验 scheme。
	return NewDOHResolver(endpoint, opts...)
}

// EndpointUsesHTTP 报告按该端点构造的解析器是否经 HTTP 发出查询（即 DoH）。
//
// 只有经 HTTP 的解析方式才有「经代理」这一出网路径：明文 DNS 走 UDP，
// 系统解析走平台 API，两者都不受进程代理环境变量影响。调用者（例如连通性
// 探测）据此决定是否需要探测代理路径。
//
// 写法非法时 panic，与 NewResolver 一致——调用者不必先自行校验一遍。
func EndpointUsesHTTP(endpoint string) bool {
	scheme, _ := parseScheme(endpoint)
	return scheme == schemeHTTP || scheme == schemeHTTPS
}

// parseScheme 解析端点地址并校验 scheme。
//
// 写法非法时 panic：判断端点性质与构造解析器共用这一套校验，使两者对同一
// 字符串给出一致的结论。
func parseScheme(endpoint string) (string, *url.URL) {
	u, err := url.Parse(endpoint)
	if err != nil {
		panic(fmt.Sprintf("pixiv: dns: 端点地址 %q 无法解析: %v", endpoint, err))
	}
	switch u.Scheme {
	case schemeHTTP, schemeHTTPS, schemeDNS:
		return u.Scheme, u
	case "":
		panic(fmt.Sprintf("pixiv: dns: 端点地址 %q 缺少 scheme: %s", endpoint, endpointHint))
	default:
		panic(fmt.Sprintf("pixiv: dns: 端点地址 %q 的 scheme %q 无效: %s", endpoint, u.Scheme, endpointHint))
	}
}

// newDNSResolver 处理 dns 方式的端点：主机为空表示系统解析，否则是明文 DNS。
func newDNSResolver(endpoint string, u *url.URL) Resolver {
	if u.Opaque != "" {
		// dns:1.1.1.1 中的 1.1.1.1 会进入 Opaque 而非 Host。若按「主机为空
		// 即系统解析」处理，用户会以为指定了服务器而实际走了系统解析。
		panic(fmt.Sprintf("pixiv: dns: 端点地址 %q 缺少 //: 指定服务器写成 dns://<ip>[:port]，用系统解析写成 dns:", endpoint))
	}
	if u.Fragment != "" {
		// #type 是 DoH 的查询方式声明，明文 DNS 没有对应概念。留下一个不会
		// 生效的声明比报错更糟。
		panic(fmt.Sprintf("pixiv: dns: 端点地址 %q: dns 方式不接受 fragment: #type 是 DoH 的查询方式声明，明文 DNS 没有对应概念", endpoint))
	}
	if u.Host == "" && u.Path == "" {
		return NewSystemResolver()
	}

	server := u.Host
	if server == "" {
		server = u.Path
	}
	host := u.Hostname()
	// 只接受 IP 字面量：本变量的用途是绕开系统 DNS，填主机名等于又依赖
	// 一次解析，Dial 回调里没有可用于解析它的东西。
	if net.ParseIP(host) == nil {
		panic(fmt.Sprintf("pixiv: dns: 端点地址 %q 的服务器 %q 不是 IP 字面量: 写成 dns://<ip>[:port]", endpoint, server))
	}
	port := u.Port()
	if port == "" {
		port = defaultDNSPort
	}
	// JoinHostPort 会为 IPv6 字面量补上方括号，还原成拨号地址。
	return newPlainResolver(net.JoinHostPort(host, port))
}

// plainResolver 是明文 DNS 解析器：把查询发往固定的服务器地址。
type plainResolver struct {
	resolver *net.Resolver
}

// newPlainResolver 构造明文 DNS 解析器，server 是 host:port 形式的拨号地址。
func newPlainResolver(server string) *plainResolver {
	// 复用标准库的 DNS 客户端：UDP 查询与响应截断后的 TCP 重试都由它处理，
	// 本包只需要把查询拨向指定服务器。PreferGo 是必需的，否则 Dial 不会被
	// 使用（系统解析在 Windows 上走系统 API）。
	return &plainResolver{
		resolver: &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, network, server)
			},
		},
	}
}

// Resolve implements Resolver
func (r *plainResolver) Resolve(ctx context.Context, host string) (ip []net.IP, err error) {
	addrs, err := r.resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		ips = append(ips, a.IP)
	}
	return ips, nil
}
