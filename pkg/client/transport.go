package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
)

// DefaultTransport 是新建客户端的默认传输，替换它可在进程级改变新建客户端的传输行为。
var DefaultTransport http.RoundTripper = &AutoTransport{}

// defaultBaseTransport 返回提供拨号、代理与 DNS 的底层传输。
func defaultBaseTransport() *http.Transport {
	return http.DefaultTransport.(*http.Transport).Clone()
}

// AutoTransport 自动为请求选择最合适的传输方式，尽力而为。
//
// 它不承诺内部实现：可能使用路由传输，也可能不使用；可能记住上次成功的
// 方式，也可能不。调用者不应依赖它的选择过程，只应依赖「请求最终被正确发出，
// 或返回错误」这一外部结果。
type AutoTransport struct {
	// Base 提供拨号、代理与 DNS。置空时使用进程默认传输；应在首次使用前设置。
	//
	// 显式设置 Base 表示调用者指定了自己希望的管道，库据此行事：ECH 主机若因
	// 该传输的代理而无法直连，会返回错误而不是悄悄改用普通连接。
	// 未设置时 base 由库自建，其代理仅来自进程环境变量，ECH 路由会忽略它。
	Base http.RoundTripper

	once   sync.Once
	base   http.RoundTripper
	routed http.RoundTripper
}

// RoundTrip implements http.RoundTripper
func (t *AutoTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.once.Do(func() {
		var implicit bool
		t.base = t.Base
		if t.base == nil {
			// 调用者未提供底层传输：base 由本库自建，其代理来自进程环境变量
			// （http.DefaultTransport 的 Proxy 是 ProxyFromEnvironment）。
			// 那不是调用者的意图，只是环境泄漏，故 ECH 路由可忽略它。
			implicit = true
			t.base = defaultBaseTransport()
		}
		t.routed = newRoutedTransport(t.base, implicit)
	})
	resp, err := t.routed.RoundTrip(req)
	// body 无法重建的请求不可原样重发（body 已被首次尝试消费），失败直接向上传播；
	// 其余请求在首选方式不可用时继续尝试其余方式。
	if err == nil || (req.Body != nil && req.GetBody == nil) {
		return resp, err
	}
	if !hasRoutedWay(req.URL.Hostname()) {
		// 该主机没有特殊方式，路由传输用的就是 base，重试没有意义。
		return resp, err
	}
	// 保存首选方式的错误，用于全部失败时聚合呈现。
	var routedErr = err
	if req.Body != nil {
		// 用 GetBody 重建 body 再重试，与标准库处理 307/308 重定向的方式一致；
		// 克隆请求以免修改调用者的请求。
		body, bodyErr := req.GetBody()
		if bodyErr != nil {
			return resp, routedErr
		}
		retryReq := req.Clone(req.Context())
		retryReq.Body = body
		req = retryReq
	}
	// 特殊方式不可用时回落到常规连接：ECH 不适用于所有主机与网络环境
	// （例如目标不在 Cloudflare 之后），此时仍有常规途径可用。
	resp, err = t.base.RoundTrip(req)
	if err == nil {
		return resp, nil
	}
	// 全部方式失败：聚合各方式的错误，调用者能看到每种方式的失败原因。
	return resp, errors.Join(routedErr, err)
}

// hasRoutedWay 报告该主机是否配有针对性的特殊连接方式。
func hasRoutedWay(host string) bool {
	if _, ok := noSNIHostnames[host]; ok {
		return true
	}
	_, ok := echHostnames[host]
	return ok
}

// noSNIHostnames 列出需要不发送 SNI 才能取回内容的主机。
//
// 这是本库掌握的 pixiv 主机布局知识，不对外暴露为选项：调用者无法观测
// pixiv 侧的变化，做成配置等于把一个无法完成的任务转嫁出去。
var noSNIHostnames = map[string]struct{}{
	// Pixiv 自有源站，接受不携带 SNI 的握手，配合 Referer 即可取图。
	"i.pximg.net": {},
}

// echHostnames 列出托管在 Cloudflare、可经 ECH 直连的主机。
//
// 与 noSNIHostnames 一样属于库掌握的 pixiv 主机布局知识，不对外暴露为选项。
//
// 这些主机按 SNI 封锁，且服务端已不接受 SNI 与 Host 不匹配的请求
// （见 docs/direct-connection.rst）。ECH 把真实域名加密在内层，
// 中间设备只能看到外层名，因此直连可用。
//
// 只列出确实托管在 Cloudflare 的主机：ECH 只对这类主机适用，对不在
// Cloudflare 之后的主机（如 i.pximg.net）其证书与 ECH 外层名不匹配。
var echHostnames = map[string]struct{}{
	"www.pixiv.net":     {},
	"app-api.pixiv.net": {},
}

// NewRoutedTransport 按请求主机把请求交给适合该主机的传输。
//
// 主机清单由库持有：调用者不需要知道 pixiv 有哪些主机、哪个主机适用哪种方式，
// 也不必自己维护这份清单。base 提供拨号、代理与 DNS，路由在其之上进行。
//
// 调用者显式提供 base 即为指定了自己希望的管道，其代理设置会被尊重：
// ECH 主机在存在代理时无法直连，此时返回错误而不是悄悄改用普通连接。
//
// 需要 TLS 层能力的主机只能建立在 *http.Transport 之上；base 不是
// *http.Transport 时（例如调用者注入了自己的 RoundTripper），所有主机
// 都按 base 的常规方式访问。
func NewRoutedTransport(base http.RoundTripper) http.RoundTripper {
	// 调用者显式提供了 base，其代理设置是明确的意图，不予忽略。
	return newRoutedTransport(base, false)
}

type routedTransport struct {
	base   http.RoundTripper
	routes map[string]http.RoundTripper
}

func newRoutedTransport(base http.RoundTripper, implicitBase bool) *routedTransport {
	if base == nil {
		base = defaultBaseTransport()
		implicitBase = true
	}
	var ret = &routedTransport{
		base:   base,
		routes: make(map[string]http.RoundTripper, len(noSNIHostnames)+len(echHostnames)),
	}
	if b, ok := base.(*http.Transport); ok {
		for host := range noSNIHostnames {
			ret.routes[host] = newNoSNITransport(b, host)
		}
		// 每个 ECH 主机一份传输：ECH 配置是每 Transport 一份的字段，
		// 且各主机的自举与轮换互相独立。
		for host := range echHostnames {
			ret.routes[host] = newECHTransportState(b, implicitBase)
		}
	}
	return ret
}

// RoundTrip implements http.RoundTripper
func (t *routedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var rt, ok = t.routes[req.URL.Hostname()]
	if !ok {
		return t.base.RoundTrip(req)
	}
	resp, err := rt.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	// 这些主机的重定向有时会降级为 http，改为 https 以免明文传输。
	if to := resp.Header.Get("Location"); strings.HasPrefix(to, "http:") {
		resp.Header.Set("Location", "https:"+to[len("http:"):])
	}
	return resp, nil
}

// noSNIServerName 用于抑制 SNI 扩展。
//
// crypto/tls 对 IP 字面量不生成 SNI 扩展（RFC 6066），因此把 ServerName 设为
// 任意 IP 字面量即可不发送 SNI，同时保留与代理共存的能力——DialTLSContext
// 只对 non-proxied 请求生效，经代理时会被静默忽略，故不使用它。
const noSNIServerName = "0.0.0.0"

// NewNoSNITransport 返回一个不发送 SNI 的传输原语：在 base 之上叠加
// 「握手时不携带 SNI」这一种连接能力。
//
// 它不含主机判断，也不含环境判断：单独使用它的人需要自己决定何时用它。
// 若需要「按主机自动选用」，用 [NewRoutedTransport]。
//
// 不发送 SNI 后标准库无法自动按主机名校验证书，因此本原语改为自行校验证书链。
// 由于原语不知道目标主机名，单独使用时只校验证书链；由 [NewRoutedTransport]
// 使用时还会校验证书主机名。需要更严格的校验时，调用者可在返回的传输上
// 自行设置 VerifyPeerCertificate。
//
// 返回的传输克隆自 base，因此可与其他原语嵌套组合，且不改变调用者的 base。
func NewNoSNITransport(base *http.Transport) *http.Transport {
	return newNoSNITransport(base, "")
}

func newNoSNITransport(base *http.Transport, host string) *http.Transport {
	if base == nil {
		base = defaultBaseTransport()
	}
	var t = base.Clone()
	var cfg *tls.Config
	if t.TLSClientConfig == nil {
		cfg = new(tls.Config)
	} else {
		cfg = t.TLSClientConfig.Clone()
	}
	cfg.ServerName = noSNIServerName
	if !cfg.InsecureSkipVerify {
		cfg.InsecureSkipVerify = true
		// 调用者原有的校验回调仍然生效，叠加而非替换，以便组合时不丢失调用者的要求。
		cfg.VerifyPeerCertificate = chainVerifyPeerCertificate(
			verifyPeerCertificate(host, cfg.RootCAs),
			cfg.VerifyPeerCertificate,
		)
	}
	t.TLSClientConfig = cfg
	t.DialContext = resolverDialContext(t.DialContext, host)
	return t
}

// chainVerifyPeerCertificate 依次执行两个校验回调，任一失败即返回该错误。
func chainVerifyPeerCertificate(
	first, second func([][]byte, [][]*x509.Certificate) error,
) func([][]byte, [][]*x509.Certificate) error {
	if second == nil {
		return first
	}
	return func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
		if err := first(rawCerts, verifiedChains); err != nil {
			return err
		}
		return second(rawCerts, verifiedChains)
	}
}

// verifyPeerCertificate 校验服务器证书链，并在已知主机名时校验主机名。
//
// 这是 InsecureSkipVerify 的替代校验：不发送 SNI 时标准库无法自动完成这一步。
func verifyPeerCertificate(host string, roots *x509.CertPool) func([][]byte, [][]*x509.Certificate) error {
	return func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
		if len(rawCerts) == 0 {
			return errors.New("pixiv: client: 服务器未提供证书")
		}
		var certs = make([]*x509.Certificate, len(rawCerts))
		for i, raw := range rawCerts {
			var cert, err = x509.ParseCertificate(raw)
			if err != nil {
				return fmt.Errorf("pixiv: client: 无法解析服务器证书: %w", err)
			}
			certs[i] = cert
		}
		var opts = x509.VerifyOptions{
			Roots:         roots,
			Intermediates: x509.NewCertPool(),
		}
		for _, cert := range certs[1:] {
			opts.Intermediates.AddCert(cert)
		}
		if _, err := certs[0].Verify(opts); err != nil {
			return fmt.Errorf("pixiv: client: 服务器证书不可信: %w", err)
		}
		if host == "" {
			return nil
		}
		if err := certs[0].VerifyHostname(host); err != nil {
			return fmt.Errorf("pixiv: client: 服务器证书与主机 %s 不匹配: %w", host, err)
		}
		return nil
	}
}

// resolverDialContext 让拨号使用请求上下文中注入的解析器解析目标主机。
//
// host 非空时只对该主机生效（路由传输知道自己选用该传输的主机名）；
// host 为空时按拨号目标判断（原语单独使用，不含主机判断）。
// 两种情况都不解析 IP 字面量，因此经代理时拨号的是代理地址、由代理解析目标主机；
// 但代理地址本身为主机名时，同样会经注入的解析器解析。
// 上下文中没有解析器时回落到原拨号函数，即使用系统解析。
// 解析失败与连接失败给出不同措辞，调用者据此区分 DNS 污染与网络封锁。
func resolverDialContext(
	dial func(ctx context.Context, network, addr string) (net.Conn, error),
	host string,
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
