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
	// resolver 是 DoH 查询与直连拨号使用的解析器，与运行时同源
	// （由环境变量播种）。
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
// 运行时 DoH 查询经 http.DefaultClient 发出（遵循 HTTPS_PROXY），因此
// 「直连 / 经代理」两个分支都用注入了受控 client 的解析器完整复现查询，
// 得到与运行时两种环境对应的结果。viaProxy 为 true 且未配置代理时快速失败
// ——静默按直连处理会产出误导性的「DoH 无需代理」结论。
func (p liveProber) ProbeDoH(ctx context.Context, endpoint, host string, viaProxy bool) ([]net.IP, error) {
	hc := &http.Client{}
	if viaProxy {
		if p.proxy == nil {
			return nil, fmt.Errorf("pixiv: connectivity: 未配置代理，无法探测 DoH 的代理路径")
		}
		// 受控分支：强制经代理，对应「运行时环境变量指向可用代理」的情形。
		hc.Transport = &http.Transport{Proxy: func(*http.Request) (*url.URL, error) { return p.proxy, nil }}
	} else {
		// 受控分支：禁用代理，使直连查询不受进程环境变量影响。
		hc.Transport = &http.Transport{Proxy: nil}
	}
	// 受控分支注入 client 以复现直连 / 经代理两种环境；
	// 编码方式仍由端点 URL 的 fragment 决定，与运行时同一来源。
	r := dns.NewDOHResolver(endpoint, dns.WithHTTPClient(hc))
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
// 与运行时图片通道共享同一构造：NewNoSNITransport(nil) 自建 base 并设置
// 库内的解析接缝，解析器经 WithDNSResolver 注入。
func (p liveProber) ProbeNoSNI(ctx context.Context, host string) error {
	c := client.New(
		client.WithTransport(client.NewNoSNITransport(nil)),
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
