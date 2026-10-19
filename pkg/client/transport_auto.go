package client

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
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
				// 响应不可用，关闭它以免连接与正文泄漏给一个没人会消费的应答；
				// 关闭失败也是调用者可见的诊断信息。
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
	// caller 是调用者显式提供的底层传输；nil 表示由本库自建。
	// 自定义通道的装配需要它来区分代理的来源（见 [echTransport.implicitProxy]）。
	caller, _ := t.Base.(*http.Transport)
	var implicit = caller == nil
	t.base = t.Base
	if t.base == nil {
		// 调用者未提供底层传输：base 由本库自建，其代理来自进程环境变量
		// （http.DefaultTransport 的 Proxy 是 ProxyFromEnvironment）。
		// 那不是调用者的意图，只是环境泄漏，故 ECH 路由可忽略它。
		t.base = defaultBaseTransport()
	}
	if b, ok := t.base.(*http.Transport); ok {
		t.ech = newECHTransportState(b, implicit)
		// no-SNI 的连接经带目标别名的 base 装配：请求 Host:www.pixiv.net 时
		// 拨号解析到 pixiv.net 源站（Cloudflare 拒绝 no-SNI 握手，只有源站接受）。
		// 与诊断工具用的是同一个导出构造，因此别名与代理的规则只有一处：
		// 未提供 base 即隐式代理（需要别名的主机直连），提供了 base 即显式代理
		// （那些主机快速失败）。ECH 仍经 b 解析 www.pixiv.net，两类目标互不干扰。
		t.nosni = NewNoSNIAliasTransport(caller)
		return
	}
	// base 不是 *http.Transport：ECH 与不发送 SNI 都只能由 TLSClientConfig 表达，
	// 库不去猜测调用者传输的语义，所有主机都按它的常规方式访问。
	t.ech, t.nosni = nil, nil
}

// NewNoSNIAliasTransport 返回库为 API 主机装配的、带源站别名的不发送 SNI 的通道。
//
// 它是 [NewECHTransport] 的对应物：调用者不必知道哪些主机需要别名，也不必自己组合
// 别名与代理的处理——那是库持有的知识（见 [NoSNIHostTarget]）。诊断工具需要按运行时的
// 方式访问这些主机时用它，而不是自行组合原语，规则因此只有一份、不会与运行时漂移。
//
// base 传 nil 表示由本库自建（[AutoTransport] 未设置 Base 时的情形）：其代理只来自
// 进程环境变量，不是调用者的意图，故需要别名的主机忽略代理、直连，使别名生效。
// 调用者传入 base 时其中的代理即视为显式指定：那些主机返回错误说明「需要直连才能
// 使用源站别名」，而不是把请求交给代理解析到别名之外的对端。
//
// 不需要别名的主机（图片主机）经本通道时仍按 base 的代理发出，且不发送 SNI。
func NewNoSNIAliasTransport(base *http.Transport) http.RoundTripper {
	// 调用者显式提供 base 时其中的代理是明确的意图，不予忽略（与 NewECHTransport 一致）。
	return newAliasedNoSNITransport(base, base == nil, noSNIHostTarget)
}

// aliasedNoSNITransport 承载「需要源站别名的主机」的不发送 SNI 的通道。
//
// 别名由 [resolverDialContext] 在拨号时施加（见 [NewHostAliasTransport]），因此只在
// 直连时生效：经代理时拨号目标是代理地址，别名分支不进入，请求会被代理解析到别名
// 之外的对端——那正是本要避开的一方（www.pixiv.net 的别名 pixiv.net 是源站，别名
// 之外是 Cloudflare，而后者不接受不携带 SNI 的握手）。故本传输按代理的来源处理
// 需要别名的主机，与 [echTransport.implicitProxy] 同一套语义：
//
//   - 隐式代理（base 由本库自建）：代理只来自进程环境变量，不是调用者的意图，而这条
//     通道的意义正是经别名直达源站，故对这些主机忽略代理、直连。
//   - 显式代理（调用者提供了 base）：代理是明确的指令，别名无法生效，故对这些主机
//     快速失败并说明冲突，而不是发出一次注定落到错误对端的请求。
//
// 不需要别名的主机不受本层处理：它们在直连与经代理下落到同一对端，保持「经代理仍
// 不发送 SNI」的能力（图片主机即为此类）。
type aliasedNoSNITransport struct {
	// base 是叠加了目标别名与「不发送 SNI」两种能力的传输。
	base http.RoundTripper
	// alias 是需要源站别名的主机；其键集决定哪些主机受本层处理。
	alias map[string]string
	// implicitProxy 见类型文档。
	implicitProxy bool
	// proxy 是 base 实际使用的代理函数，用于判断本次请求是否会经代理发出。
	proxy func(*http.Request) (*url.URL, error)
}

// newAliasedNoSNITransport 装配「需要源站别名的主机」的不发送 SNI 的通道。
//
// implicitProxy 表示 base 由本库自建（其代理只来自进程环境变量），见
// [aliasedNoSNITransport]。base 为空时同样按自建处理。
func newAliasedNoSNITransport(
	base *http.Transport, implicitProxy bool, alias map[string]string,
) *aliasedNoSNITransport {
	if base == nil {
		base = defaultBaseTransport()
		implicitProxy = true
	}
	var physical = base.Clone()
	if implicitProxy && physical.Proxy != nil {
		// 隐式代理不是调用者的意图，而需要别名的主机在代理下到不了源站：
		// 忽略它使别名重新生效。
		physical.Proxy = proxyIgnoringHosts(physical.Proxy, alias)
	}
	return &aliasedNoSNITransport{
		base:          NewNoSNITransport(NewHostAliasTransport(physical, alias)),
		alias:         alias,
		implicitProxy: implicitProxy,
		proxy:         physical.Proxy,
	}
}

// RoundTrip implements http.RoundTripper
func (t *aliasedNoSNITransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if _, ok := t.alias[req.URL.Hostname()]; ok && t.usesProxy(req) {
		// 继续下去只会把请求交给代理解析，从而落到别名之外的对端，故明确报错，
		// 使调用者知道自己指定的代理与源站别名冲突，而不是看到一次无意义的应答。
		return nil, fmt.Errorf(
			"pixiv: client: 主机 %s 需要直连才能使用源站别名，但底层传输指定了代理；"+
				"请为该主机去掉代理，或改用其它传输方式", req.URL.Hostname())
	}
	return t.base.RoundTrip(req)
}

// CloseIdleConnections 关闭空闲连接，使 [http.Client.CloseIdleConnections] 对本通道
// 仍然有效——否则调用者只能经 http.RoundTripper 使用它，无从释放连接。
func (t *aliasedNoSNITransport) CloseIdleConnections() {
	if c, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		c.CloseIdleConnections()
	}
}

// usesProxy 报告该请求是否会经代理发出。
//
// 隐式代理已对需要别名的主机被忽略（见 [newAliasedNoSNITransport]），故一律按不走
// 代理处理。
func (t *aliasedNoSNITransport) usesProxy(req *http.Request) bool {
	if t.implicitProxy || t.proxy == nil {
		return false
	}
	proxyURL, err := t.proxy(req)
	// 代理函数出错时按「不走代理」处理：让请求正常发出，由底层传输自己报告
	// 代理配置的问题（与 [echTransport.usesProxy] 一致）。
	return err == nil && proxyURL != nil
}

// proxyIgnoringHosts 返回一个代理函数：hosts 的键集列出的主机不使用代理（直连），
// 其余主机沿用原函数。
func proxyIgnoringHosts(
	proxy func(*http.Request) (*url.URL, error), hosts map[string]string,
) func(*http.Request) (*url.URL, error) {
	return func(req *http.Request) (*url.URL, error) {
		if _, ok := hosts[req.URL.Hostname()]; ok {
			return nil, nil
		}
		return proxy(req)
	}
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
