package client

import (
	"net/http"
	"strings"
)

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
