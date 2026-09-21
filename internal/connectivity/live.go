package connectivity

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"

	"github.com/NateScarlet/pixiv/pkg/client"
	"github.com/NateScarlet/pixiv/pkg/client/dns"
	"github.com/tidwall/gjson"
)

// Prober 提供单项探测能力，是网络实现的接缝；测试注入 fake。
// 定义见 connectivity.go。
var _ Prober = liveProber{}

// queryDoH 用指定端点发起一次 DoH 查询，返回 A 记录的地址。
//
// proxy 非 nil 时强制经该代理发出：DoH 在运行时经 http.DefaultClient 发出、
// 遵循 HTTPS_PROXY，显式设置代理即对应「运行时代理路径」的行为；
// nil 时不设置代理函数，但由于探测不应受环境变量影响，调用方传入的
// transport 已禁用代理。应答格式与库内解析器一致（Google JSON API）。
func queryDoH(ctx context.Context, endpoint, host string, proxy *url.URL) ([]net.IP, error) {
	t := defaultTransport()
	if proxy != nil {
		t.Proxy = func(*http.Request) (*url.URL, error) { return proxy, nil }
	} else {
		t.Proxy = nil
	}
	c := &http.Client{Transport: t}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	q := req.URL.Query()
	q.Set("name", host)
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Accept", "application/dns-json")

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("查询 DoH 端点 %s 失败: %w", endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DoH 端点 %s 返回状态码 %d", endpoint, resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 DoH 应答失败: %w", err)
	}
	var ips []net.IP
	gjson.ParseBytes(data).Get("Answer.#(type==1)#.data").ForEach(func(_, v gjson.Result) bool {
		if ip := net.ParseIP(v.String()); ip != nil {
			ips = append(ips, ip)
		}
		return true
	})
	return ips, nil
}

// dialViaResolver 返回经解析器解析主机名的拨号函数：IP 字面量不解析，
// 主机名解析失败时给出指明排查方向的错误。
//
// 语义与库内 resolverDialContext 一致；不复用后者是因为它是未导出的
// 内部装配，而探测工具只需这一层薄包装。
func dialViaResolver(resolver dns.Resolver) func(ctx context.Context, network, addr string) (net.Conn, error) {
	var d net.Dialer
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("pixiv: connectivity: 无法解析目标地址 %q: %w", addr, err)
		}
		if net.ParseIP(host) != nil {
			return d.DialContext(ctx, network, addr)
		}
		ips, err := resolver.Resolve(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("pixiv: connectivity: 解析主机 %s 失败（DoH 端点不可达？）: %w", host, err)
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("pixiv: connectivity: 主机 %s 没有解析结果", host)
		}
		var lastErr error
		for _, ip := range ips {
			conn, err := d.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		return nil, fmt.Errorf("pixiv: connectivity: 主机 %s 已解析但无法建立连接: %w", host, lastErr)
	}
}

// resolverTransport 把解析器注入请求上下文，供自行拨号的传输取用。
//
// 语义与库内同名装配一致；独立实现以保持本包不依赖 client 的内部装配。
type resolverTransport struct {
	wrapped  http.RoundTripper
	resolver dns.Resolver
}

// RoundTrip implements http.RoundTripper.
func (t *resolverTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := context.WithValue(req.Context(), resolverContextKey{}, t.resolver)
	return t.wrapped.RoundTrip(req.WithContext(ctx))
}

// resolverContextKey 是解析器在请求上下文中的键。
type resolverContextKey struct{}

// withResolver 给传输包一层解析器注入；nil 解析器原样返回。
func withResolver(rt http.RoundTripper, r dns.Resolver) http.RoundTripper {
	if r == nil {
		return rt
	}
	return &resolverTransport{wrapped: rt, resolver: r}
}

// liveProber 是 Prober 的真实网络实现，复用库导出的传输原语。
//
// 各探测一次性使用独立传输，不复用连接，避免前一项探测的连接状态
// 影响后一项的结果。
type liveProber struct {
	// proxy 是探测代理路径时使用的代理地址；nil 表示未配置代理，
	// 此时经代理的探测快速失败。
	proxy *url.URL
	// resolver 是 DoH 查询与直连拨号使用的解析器。
	resolver dns.Resolver
}

// NewLiveProber 返回真实网络的探测器。
//
// proxy 是代理路径探测使用的地址（来自环境配置，nil 表示未配置代理）；
// resolver 由最外层装配注入：它是环境配置（PIXIV_DNS_QUERY_URL）的生效值，
// 本包不自行读取环境。
func NewLiveProber(proxy *url.URL, resolver dns.Resolver) Prober {
	return liveProber{proxy: proxy, resolver: resolver}
}

// ProbeDoH implements Prober.
//
// viaProxy 为 true 时强制经装配时注入的代理发出；未配置代理时快速失败
// ——静默按直连处理会产出误导性的「DoH 无需代理」结论。
func (p liveProber) ProbeDoH(ctx context.Context, endpoint, host string, viaProxy bool) ([]net.IP, error) {
	if viaProxy && p.proxy == nil {
		return nil, fmt.Errorf("pixiv: connectivity: 未配置代理，无法探测 DoH 的代理路径")
	}
	if viaProxy {
		return queryDoH(ctx, endpoint, host, p.proxy)
	}
	// 直连查询走独立的禁用代理的传输，不受环境变量影响。
	return queryDoH(ctx, endpoint, host, nil)
}

// ProbeHTTPS implements Prober.
//
// viaProxy 为 true 时强制经装配时注入的代理发出（调用者的明确意图，
// 等价于 AutoTransport 显式提供 Base 的语义）；为 false 时禁用代理，
// 使探测结果不受环境变量影响。
func (p liveProber) ProbeHTTPS(ctx context.Context, rawURL string, viaProxy bool) error {
	if viaProxy && p.proxy == nil {
		return fmt.Errorf("pixiv: connectivity: 未配置代理，无法探测经代理的路径")
	}
	t := defaultTransport()
	if viaProxy {
		t.Proxy = func(*http.Request) (*url.URL, error) { return p.proxy, nil }
	} else {
		t.Proxy = nil
	}
	return probeOnce(ctx, t, rawURL)
}

// ProbeECH implements Prober.
//
// 直连施加 ECH：显式禁用代理（ECH 的意义正是绕开按 SNI 的封锁，
// 经代理时封锁本已被绕过，测不出 ECH 是否生效），拨号经注入的解析器
// 解析目标主机以避开系统解析污染，自举与配置轮换由传输原语自理。
func (p liveProber) ProbeECH(ctx context.Context, host string) error {
	t := defaultTransport()
	t.Proxy = nil
	t.DialContext = dialViaResolver(p.resolver)
	rt := client.NewECHTransport(t)
	return probeOnce(ctx, rt, fmt.Sprintf("https://%s/", host))
}

// ProbeNoSNI implements Prober.
//
// 直连不发送 SNI：显式禁用代理（经代理时封锁本已被绕过），拨号经注入的
// 解析器解析目标主机——「解析被污染」与「无 SNI 直连被封」是两种不同的
// 故障，前者由 DoH 项与解析结果对照表说明。
func (p liveProber) ProbeNoSNI(ctx context.Context, host string) error {
	t := defaultTransport()
	t.Proxy = nil
	t.DialContext = dialViaResolver(p.resolver)
	rt := client.NewNoSNITransport(t)
	return probeOnce(ctx, rt, fmt.Sprintf("https://%s/", host))
}

// probeOnce 用给定传输发出一次 GET 请求并丢弃响应体。
//
// 成功标准是「收到 HTTP 应答」：连接层可用即成功，业务状态码（如图片
// 主机对无 Referer 请求的 403）不构成探测失败。
func probeOnce(ctx context.Context, rt http.RoundTripper, rawURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Transport: rt}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, err = io.Copy(io.Discard, resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}
	return nil
}

// defaultTransport 返回一份独立的底层传输。
//
// 每次探测使用独立传输：不复用连接，避免前一项探测的连接状态影响
// 后一项的结果。
func defaultTransport() *http.Transport {
	return http.DefaultTransport.(*http.Transport).Clone()
}
