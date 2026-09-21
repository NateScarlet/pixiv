package client

import (
	"errors"
	"net/http"
	"strings"
)

// apiHostnames 列出应经 api 通道访问的主机。
//
// 这些主机托管在 Cloudflare、按 SNI 封锁，且服务端已不接受 SNI 与 Host 不匹配的
// 请求（见 docs/direct-connection.rst），因此适合经 ECH 直连：ECH 把真实域名加密
// 在内层，中间设备只能看到外层名。ECH 只对托管在 Cloudflare 的主机适用，对不在
// 其后的主机（例如 i.pximg.net）其证书与 ECH 的外层名不匹配。
//
// 与 [imageHostnames] 一样属于库掌握的 pixiv 主机布局知识，不对外暴露为选项：
// 调用者无法观测 pixiv 侧的变化，做成配置等于把一个无法完成的任务转嫁出去。
var apiHostnames = map[string]struct{}{
	"www.pixiv.net":     {},
	"app-api.pixiv.net": {},
}

// imageHostnames 列出应经 image 通道访问的主机。
//
// Pixiv 自有源站（不在 Cloudflare 之后），接受不携带 SNI 的握手，配合 Referer
// 即可取图。
var imageHostnames = map[string]struct{}{
	"i.pximg.net": {},
}

// NewRoutedTransport 按请求主机把请求交给适合该主机的通道。
//
// 主机清单由库持有：调用者不需要知道 pixiv 有哪些主机、哪个主机该用哪种方式，
// 也不必自己维护这份清单。两个通道由调用者提供，因此可以各自带上自己的拨号、
// 代理与 DNS 设置——API 主机与图片主机需要不同的连接方式，也就需要不同的传输。
// 本函数只做路由，不代为构造传输；需要自动装配时用 [AutoTransport]。
//
// image 通道按「不发送 SNI」的传输理解（通常由 [NewNoSNITransport] 构造）：它的
// 握手不含主机名，标准库因此无法按主机名校验证书，本传输在收到响应后补齐这一步。
// api 通道不做此假定，其证书由该传输自行校验。
//
// 未列入清单的主机交给 api，即 api 兼作默认通道。
func NewRoutedTransport(api, image http.RoundTripper) http.RoundTripper {
	if api == nil {
		panic("pixiv: client: NewRoutedTransport 的 api 传输为空: 请显式提供，或改用 AutoTransport")
	}
	if image == nil {
		panic("pixiv: client: NewRoutedTransport 的 image 传输为空: 请显式提供，或改用 AutoTransport")
	}
	return newRoutedTransport(api, image)
}

// route 是一条主机对应的分派目标。
type route struct {
	rt http.RoundTripper
	// noSNI 表示该通道的握手不含主机名，因此证书的主机名校验要由本传输补齐。
	noSNI bool
}

// routedTransport 按请求主机在调用者提供的通道之间分派请求。
type routedTransport struct {
	// fallback 承接未列入清单的主机。
	fallback http.RoundTripper
	routes   map[string]route
}

// newRoutedTransport 构造路由：未列入清单的主机交给 api，即 api 兼作默认通道。
func newRoutedTransport(api, image http.RoundTripper) *routedTransport {
	return newRoutedTransportWithFallback(api, api, image)
}

// newRoutedTransportWithFallback 构造路由，并显式指定未列入清单的主机的通道。
//
// [AutoTransport] 用它把默认通道与 API 通道分开：API 通道叠加了 ECH，而 ECH 只
// 适用于托管在 Cloudflare 的 pixiv 主机，对调用者指定的其他地址（例如镜像）
// 不适用。
func newRoutedTransportWithFallback(fallback, api, image http.RoundTripper) *routedTransport {
	var ret = &routedTransport{
		fallback: fallback,
		routes:   make(map[string]route, len(apiHostnames)+len(imageHostnames)),
	}
	for host := range apiHostnames {
		ret.routes[host] = route{rt: api}
	}
	for host := range imageHostnames {
		ret.routes[host] = route{rt: image, noSNI: true}
	}
	return ret
}

// RoundTrip implements http.RoundTripper
func (t *routedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var host = req.URL.Hostname()
	r, ok := t.routes[host]
	if !ok {
		// 未列入清单的主机没有专门的接入方式，也就没有需要本层补齐的部分。
		return t.fallback.RoundTrip(req)
	}
	resp, err := r.rt.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	if r.noSNI {
		if err := verifyResponseHostname(resp, host); err != nil {
			// 响应不可用，关闭它以复用连接；关闭失败也是调用者可见的诊断信息。
			return nil, errors.Join(err, resp.Body.Close())
		}
	}
	// 这些主机的重定向有时会降级为 http，改为 https 以免明文传输。
	if to := resp.Header.Get("Location"); strings.HasPrefix(to, "http:") {
		resp.Header.Set("Location", "https:"+to[len("http:"):])
	}
	return resp, nil
}
