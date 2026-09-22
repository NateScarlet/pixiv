package client

import (
	"context"
	"fmt"
	"maps"
	"net"
	"net/http"
)

// DefaultTransport 是新建客户端的默认传输，替换它可在进程级改变新建客户端的传输行为。
var DefaultTransport http.RoundTripper = &AutoTransport{}

// defaultBaseTransport 返回提供拨号、代理与 DNS 的底层传输。
func defaultBaseTransport() *http.Transport {
	return http.DefaultTransport.(*http.Transport).Clone()
}

// NewHostAliasTransport 返回一个在拨号时把目标主机按 hostAlias 映射的底层传输：
// 请求主机的 Host/SNI 不变，但解析与拨号使用别名后的主机。
//
// 用途是「请求一个主机、连接到其源站」这类组合：例如请求 Host:www.pixiv.net，
// 但经不发送 SNI 的方式连接时落到 pixiv.net 源站的地址（Cloudflare 拒绝 no-SNI
// 握手，只有源站接受）。它自带经请求上下文注入解析器的解析，因此可被
// NewNoSNITransport 等原语组合，别名不必写进那些原语本身。
//
// 别名按请求主机精确匹配；未列入的主机照常解析自身。base 为空时使用进程默认传输。
func NewHostAliasTransport(base *http.Transport, hostAlias map[string]string) *http.Transport {
	if base == nil {
		base = defaultBaseTransport()
	}
	var t = base.Clone()
	t.DialContext = resolverDialContext(t.DialContext, "", hostAlias)
	return t
}

// NoSNIHostTarget 返回库为不发送 SNI 的连接使用的目标主机别名副本
// （www.pixiv.net → pixiv.net 源站）。供诊断工具等复现库的组合方式时读取；
// 返回副本，调用者修改不影响库。
func NoSNIHostTarget() map[string]string {
	return maps.Clone(noSNIHostTarget)
}

// resolverDialContext 让拨号使用请求上下文中注入的解析器解析目标主机。
//
// host 非空时只对该主机生效（ECH 自举知道自己要连接的主机名）；
// host 为空时按拨号目标判断（传输原语不含主机判断）。
// hostAlias 把解析目标按别名映射：请求主机仍是原词，但实际解析并拨号别名后的
// 主机——用于「请求一个主机、连接到其源站」这类组合，见 NewHostAliasTransport。
// 两种情况都不解析 IP 字面量，因此经代理时拨号的是代理地址、由代理解析目标主机；
// 但代理地址本身为主机名时，同样会经注入的解析器解析。
// 上下文中没有解析器时回落到原拨号函数，即使用系统解析。
// 解析失败与连接失败给出不同措辞，调用者据此区分 DNS 污染与网络封锁。
func resolverDialContext(
	dial func(ctx context.Context, network, addr string) (net.Conn, error),
	host string,
	hostAlias map[string]string,
) func(ctx context.Context, network, addr string) (net.Conn, error) {
	var fallback = dial
	if fallback == nil {
		var d net.Dialer
		fallback = d.DialContext
	}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		var r = dnsResolverFromContext(ctx)
		if r == nil {
			return fallback(ctx, network, addr)
		}
		var dialHost, port, err = net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("pixiv: client: 无法解析目标地址 %q: %w", addr, err)
		}
		var target = host
		if target == "" {
			target = dialHost
		}
		if dialHost != target || net.ParseIP(target) != nil {
			// 不是本连接能力负责解析的目标（例如代理地址或 IP 字面量）。
			return fallback(ctx, network, addr)
		}
		if alias, ok := hostAlias[target]; ok {
			target = alias
		}
		ip, err := r.Resolve(ctx, target)
		if err != nil {
			return nil, fmt.Errorf("pixiv: client: 解析主机 %s 失败，当前网络可能无法得到正确地址: %w", target, err)
		}
		if len(ip) == 0 {
			return nil, fmt.Errorf("pixiv: client: 主机 %s 没有解析结果，当前网络无可用途径连接该主机", target)
		}
		var lastErr error
		for _, v := range ip {
			// 逐个尝试解析结果，避免被污染或不可达的地址阻断取回。
			conn, err := fallback(ctx, network, net.JoinHostPort(v.String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		return nil, fmt.Errorf(
			"pixiv: client: 主机 %s 已解析但无法建立连接（最后尝试 %s），当前网络可能封锁了该主机: %w",
			target, ip[len(ip)-1], lastErr)
	}
}
