package client

import (
	"errors"
	"net/http"
	"sync"
)

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
