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
)

// Prober 提供单项探测能力，是网络实现的接缝；测试注入 fake，
// 真实实现见本文件。定义见 connectivity.go。
var _ Prober = liveProber{}

// liveProber 是 Prober 的真实网络实现。
//
// 探测管道全部由库的导出面构造（与运行时共享同一实现），本类型不自行
// 装配拨号、解析或传输——库的行为变化（例如解析接缝的修复）会自动传导
// 到探测结果，工具不会与运行时漂移。
type liveProber struct {
	// proxy 是探测代理路径时使用的代理地址；nil 表示未配置代理，
	// 此时经代理的探测快速失败。
	proxy *url.URL
	// resolver 是解析查询与直连拨号使用的解析器，与运行时同源
	// （由环境变量播种）。
	resolver dns.Resolver
}

// NewLiveProber 返回真实网络的探测器。
//
// proxy 是代理路径探测使用的地址（来自环境配置，nil 表示未配置代理）；
// resolver 由最外层装配注入：它是环境配置（PIXIV_DNS_QUERY_URL）的生效值，
// 本包不自行读取环境。探测本身用端点字符串重建同一解析方式，见 ProbeResolver。
func NewLiveProber(proxy *url.URL, resolver dns.Resolver) Prober {
	return liveProber{proxy: proxy, resolver: resolver}
}

// ProbeResolver implements Prober.
//
// 解析方式完全由端点字符串决定，与运行时同一来源（dns.NewResolver）：
// DoH、明文 DNS、系统解析三种写法探测的行为与运行时一致，不会漂移。
// 端点写法非法时与运行时一样 panic。
//
// DoH 查询经 http.DefaultClient 发出（遵循 HTTPS_PROXY），因此「直连 / 经代理」
// 两个分支都用注入了受控 client 的解析器完整复现查询。明文 DNS 与系统解析
// 没有可经代理的 HTTP 请求，viaProxy 对它们是调用错误，快速失败而不是静默
// 按直连处理——后者会产出误导性的「无需代理」结论。
func (p liveProber) ProbeResolver(ctx context.Context, endpoint, host string, viaProxy bool) ([]net.IP, error) {
	if !dns.EndpointUsesHTTP(endpoint) {
		if viaProxy {
			return nil, fmt.Errorf("pixiv: connectivity: 端点 %q 不经 HTTP 查询，没有可经代理的路径", endpoint)
		}
		return dns.NewResolver(endpoint).Resolve(ctx, host)
	}

	hc := &http.Client{}
	if viaProxy {
		if p.proxy == nil {
			return nil, fmt.Errorf("pixiv: connectivity: 未配置代理，无法探测解析查询的代理路径")
		}
		// 受控分支：强制经代理，对应「运行时环境变量指向可用代理」的情形。
		hc.Transport = &http.Transport{Proxy: func(*http.Request) (*url.URL, error) { return p.proxy, nil }}
	} else {
		// 受控分支：禁用代理，使直连查询不受进程环境变量影响。
		hc.Transport = &http.Transport{Proxy: nil}
	}
	// 受控分支注入 client 以复现直连 / 经代理两种环境；
	// 编码方式仍由端点 URL 的 fragment 决定，与运行时同一来源。
	r := dns.NewResolver(endpoint, dns.WithHTTPClient(hc))
	return r.Resolve(ctx, host)
}

// ProbeHTTPS implements Prober.
//
// 探测「常规连接」这一运行时回落途径。运行时自建 base 的代理来自进程环境
// 变量（设了 HTTPS_PROXY 时回落连接经代理发出），这里用显式代理函数表达
// 同一语义：viaProxy 时走装配注入的代理，否则禁用。
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
// 与运行时 API 通道共享同一构造：NewECHTransport(nil) 让原语自建 base
// （隐式代理语义：忽略环境代理、直连），解析器经 WithDNSResolver 随请求
// 注入——数据连接与自举的解析路径由库内实现决定，探测如实反映它们
// （包括尚未修复的缺陷，例如 PR #94 指出的数据连接解析）。
func (p liveProber) ProbeECH(ctx context.Context, host string) error {
	c := client.New(
		client.WithTransport(client.NewECHTransport(nil)),
		client.WithDNSResolver(p.resolver),
	)
	return probeWithClient(ctx, c, fmt.Sprintf("https://%s/", host))
}

// ProbeNoSNI implements Prober.
//
// 与运行时 no-SNI 通道共享同一构造：用带目标别名的 base（见 client.NoSNIHostTarget）
// 叠加不发送 SNI 的原语，解析器经 WithDNSResolver 注入——请求 Host:www.pixiv.net
// 时落到 pixiv.net 源站，与 AutoTransport 的 no-SNI 腿一致。
func (p liveProber) ProbeNoSNI(ctx context.Context, host string) error {
	c := client.New(
		// 与运行时 no-SNI 通道一致：请求 Host:www.pixiv.net 时拨号解析到 pixiv.net
		// 源站（Cloudflare 拒绝 no-SNI 握手，只有源站接受），见 client.NoSNIHostTarget。
		client.WithTransport(client.NewNoSNITransport(
			client.NewHostAliasTransport(nil, client.NoSNIHostTarget()),
		)),
		client.WithDNSResolver(p.resolver),
	)
	return probeWithClient(ctx, c, fmt.Sprintf("https://%s/", host))
}

// probeOnce 用给定传输发出一次 GET 请求并丢弃响应体。
//
// 成功标准是「收到 HTTP 应答」：连接层可用即成功，业务状态码（如图片
// 主机对无 Referer 请求的 403）不构成探测失败。
func probeOnce(ctx context.Context, rt http.RoundTripper, rawURL string) error {
	c := &http.Client{Transport: rt}
	return probeWithClient(ctx, c, rawURL)
}

// probeWithClient 用给定客户端发出一次 GET 请求并丢弃响应体，成功标准同上。
//
// client.Client 内嵌 http.Client，因此两者都以接口约束传入。
func probeWithClient(ctx context.Context, c interface {
	Do(req *http.Request) (*http.Response, error)
}, rawURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
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
