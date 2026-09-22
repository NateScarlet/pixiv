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

	once sync.Once
	base http.RoundTripper
	// ech 与 nosni 是 base 为 *http.Transport 时装配的自定义通道（API 主机的
	// ECH、不发送 SNI 的连接）；否则为空，所有主机都经 base 常规访问。
	ech, nosni http.RoundTripper
	// hostNames 覆盖待分派的主机清单，nil 时使用包级清单（apiHostnames 等）。
	// 作为注入点而非对外配置：测试注入自己的清单即可，改写包级全局会影响
	// 所有用例。
	hostNames *hostSets

	// prefer 按主机记录上次可用的连接方式，见 [AutoTransport.RoundTrip]。
	// 默认传输是共享的，因此用互斥保护读取与更新。
	mu     sync.Mutex
	prefer map[string]way
}

// hostSets 是主机清单；仅作为 AutoTransport 的测试注入点（见 hostNames）。
type hostSets struct {
	api, image map[string]struct{}
}

// way 标识一种连接方式，用于记忆「该主机上次用哪种方式成功」。
type way int

const (
	// wayECH 是施加 ECH 的直连，适用于托管在 Cloudflare 的主机。
	wayECH way = iota
	// wayNoSNI 是不发送 SNI 的连接，适用于 pixiv 自有源站。
	wayNoSNI
	// wayPlain 是常规连接（base）。
	wayPlain
)

// candidate 是一次待尝试的连接方式。
type candidate struct {
	id way
	rt http.RoundTripper
}

// RoundTrip implements http.RoundTripper
func (t *AutoTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.once.Do(t.setup)
	host := req.URL.Hostname()
	ways := t.ways(host)
	if pref, ok := t.preferredFor(host); ok {
		// 上次可用的方式排到最前：确认失败后不再于每个请求重复无谓的尝试。
		ways = moveWayToFront(ways, pref)
	}

	var errs []error
	for _, c := range ways {
		resp, err := c.rt.RoundTrip(req)
		if err == nil && c.id == wayNoSNI {
			// 不发送 SNI 的握手不含主机名，标准库无法按主机名校验证书，
			// 需按请求主机在收到响应后补齐（与路由层的做法一致）。
			err = verifyResponseHostname(resp, host)
			if err != nil {
				err = errors.Join(err, resp.Body.Close())
			}
		}
		if err == nil {
			t.remember(host, c.id)
			return resp, nil
		}
		errs = append(errs, err)
		// 请求体无法重建时不能再试其他方式（再试只会消费原 body）。
		if req.Body != nil && req.GetBody == nil {
			return resp, errors.Join(errs...)
		}
		if req.Body != nil {
			// 用 GetBody 重建 body 再重试，与标准库处理 307/308 重定向的方式一致。
			body, bodyErr := req.GetBody()
			if bodyErr != nil {
				return resp, errors.Join(append(errs, bodyErr)...)
			}
			retryReq := req.Clone(req.Context())
			retryReq.Body = body
			req = retryReq
		}
	}
	return nil, errors.Join(errs...)
}

// ways 按请求主机给出候选连接方式的有序列表。
//
//   - API 主机：优先 ECH，失败回落到不发送 SNI，最后常规。no-SNI 径对
//     www.pixiv.net 拨号解析到 pixiv.net 源站（见 noSNIHostTarget），ECH 仍
//     落在 Cloudflare，故两类目标互不干扰。
//   - 图片主机：不发送 SNI，最后常规。
//   - 其余主机：常规。
func (t *AutoTransport) ways(host string) []candidate {
	api, image := apiHostnames, imageHostnames
	if t.hostNames != nil {
		api, image = t.hostNames.api, t.hostNames.image
	}
	if _, ok := api[host]; ok && t.ech != nil {
		return []candidate{
			{wayECH, t.ech},
			{wayNoSNI, t.nosni},
			{wayPlain, t.base},
		}
	}
	if _, ok := image[host]; ok && t.nosni != nil {
		return []candidate{{wayNoSNI, t.nosni}, {wayPlain, t.base}}
	}
	return []candidate{{wayPlain, t.base}}
}

// setup 装配底层与自定义通道，只发生一次。
func (t *AutoTransport) setup() {
	var implicit bool
	t.base = t.Base
	if t.base == nil {
		// 调用者未提供底层传输：base 由本库自建，其代理来自进程环境变量
		// （http.DefaultTransport 的 Proxy 是 ProxyFromEnvironment）。
		// 那不是调用者的意图，只是环境泄漏，故 ECH 路由可忽略它。
		implicit = true
		t.base = defaultBaseTransport()
	}
	if b, ok := t.base.(*http.Transport); ok {
		t.ech = newECHTransportState(b, implicit)
		// no-SNI 的连接经带目标别名的 base 装配：请求 Host:www.pixiv.net 时
		// 拨号解析到 pixiv.net 源站（Cloudflare 拒绝 no-SNI 握手，只有源站接受）。
		// ECH 仍经 b 解析 www.pixiv.net，两类目标互不干扰。
		t.nosni = NewNoSNITransport(NewHostAliasTransport(b, noSNIHostTarget))
		return
	}
	// base 不是 *http.Transport：ECH 与不发送 SNI 都只能由 TLSClientConfig 表达，
	// 库不去猜测调用者传输的语义，所有主机都按它的常规方式访问。
	t.ech, t.nosni = nil, nil
}

// preferredFor 返回该主机上次可用的连接方式（若有记忆）。
func (t *AutoTransport) preferredFor(host string) (way, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	w, ok := t.prefer[host]
	return w, ok
}

// remember 记录该主机可用的连接方式。
func (t *AutoTransport) remember(host string, w way) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.prefer == nil {
		t.prefer = make(map[string]way)
	}
	t.prefer[host] = w
}

// moveWayToFront 把 id 对应的方式移到最前，其余保持原顺序；找不到则原样返回。
func moveWayToFront(ways []candidate, id way) []candidate {
	for i, c := range ways {
		if c.id == id && i > 0 {
			var out = make([]candidate, 0, len(ways))
			out = append(out, c)
			out = append(out, ways[:i]...)
			out = append(out, ways[i+1:]...)
			return out
		}
	}
	return ways
}
