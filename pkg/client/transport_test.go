package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// roundTripperFunc 便于在测试中构造传输。
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// okResponse 返回一个可读的空响应。
func okResponse(req *http.Request) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("{}")),
		Request:    req,
	}
}

// newRoutedTransportWithRoutes 是测试专用的路由构造，用于注入任意主机清单与通道，
// 从而可以在断言路由行为时不断言库持有的真实清单内容。
//
// 未列入清单的主机交给 api，与 [NewRoutedTransport] 一致。
func newRoutedTransportWithRoutes(api http.RoundTripper, routes map[string]route) *routedTransport {
	return &routedTransport{fallback: api, routes: routes}
}

// TestRoutedTransportSendsHostToMatchingTransport 断言请求到达适合该主机的
// 通道，其他主机仍走 api（api 兼作默认通道）。
func TestRoutedTransportSendsHostToMatchingTransport(t *testing.T) {
	var special, base int32
	baseRT := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&base, 1)
		return okResponse(req), nil
	})
	specialRT := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&special, 1)
		return okResponse(req), nil
	})
	rt := newRoutedTransportWithRoutes(baseRT, map[string]route{
		"special.example.com": {rt: specialRT},
	})

	req, err := http.NewRequest(http.MethodGet, "https://special.example.com/a", nil)
	require.NoError(t, err)
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, int32(1), atomic.LoadInt32(&special))
	assert.Equal(t, int32(0), atomic.LoadInt32(&base))

	req, err = http.NewRequest(http.MethodGet, "https://other.example.com/a", nil)
	require.NoError(t, err)
	resp, err = rt.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, int32(1), atomic.LoadInt32(&special), "其他主机不应触发特殊通道")
	assert.Equal(t, int32(1), atomic.LoadInt32(&base))
}

// TestRoutedTransportRewritesHTTPRedirect 断言受特殊处理的主机的明文重定向
// 被改写为 https。
func TestRoutedTransportRewritesHTTPRedirect(t *testing.T) {
	redirect := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		resp := okResponse(req)
		resp.StatusCode = http.StatusFound
		resp.Header.Set("Location", "http://special.example.com/next")
		return resp, nil
	})
	rt := newRoutedTransportWithRoutes(
		roundTripperFunc(func(req *http.Request) (*http.Response, error) { return okResponse(req), nil }),
		map[string]route{"special.example.com": {rt: redirect}},
	)
	req, err := http.NewRequest(http.MethodGet, "https://special.example.com/a", nil)
	require.NoError(t, err)
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, "https://special.example.com/next", resp.Header.Get("Location"))
}

// TestRoutedTransportKeepsHTTPSRedirect 断言本身就使用 https 的重定向不被改写。
func TestRoutedTransportKeepsHTTPSRedirect(t *testing.T) {
	redirect := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		resp := okResponse(req)
		resp.Header.Set("Location", "https://special.example.com/next")
		return resp, nil
	})
	rt := newRoutedTransportWithRoutes(
		roundTripperFunc(func(req *http.Request) (*http.Response, error) { return okResponse(req), nil }),
		map[string]route{"special.example.com": {rt: redirect}},
	)
	req, err := http.NewRequest(http.MethodGet, "https://special.example.com/a", nil)
	require.NoError(t, err)
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, "https://special.example.com/next", resp.Header.Get("Location"))
}

// TestNewRoutedTransportRoutesHostsByChannel 断言路由把库持有的两类主机
// 分派到调用者提供的两个通道，未列出的主机交给 api（api 兼作默认通道）。
//
// 断言的是路由行为本身，因此用可辨认的替身传输；库持有的真实清单内容由
// TestRoutedTransportHostListsCoverBothWays 断言。
func TestNewRoutedTransportRoutesHostsByChannel(t *testing.T) {
	var api, image http.RoundTripper = &spyTransport{}, &spyTransport{}
	rt := NewRoutedTransport(api, image)
	rs, ok := rt.(*routedTransport)
	require.True(t, ok)

	for host := range apiHostnames {
		assert.Equal(t, api, rs.routes[host].rt, "主机 %s 应走 api 通道", host)
	}
	for host := range imageHostnames {
		assert.Equal(t, image, rs.routes[host].rt, "主机 %s 应走 image 通道", host)
		assert.True(t, rs.routes[host].noSNI, "图片通道不发送 SNI，主机名校验需由路由补齐")
	}

	// 未列入清单的主机交给 api，即 api 兼作默认通道。
	assert.Equal(t, api, rs.fallback, "未列入清单的主机应交给 api 通道")
}

// TestNewRoutedTransportRequiresBothTransports 断言缺少通道时快速失败：
// 两个通道都是必需依赖，库不代为构造。
func TestNewRoutedTransportRequiresBothTransports(t *testing.T) {
	assert.Panics(t, func() { NewRoutedTransport(nil, &spyTransport{}) })
	assert.Panics(t, func() { NewRoutedTransport(&spyTransport{}, nil) })
}

// TestRoutedTransportHostListsCoverBothWays 断言库持有的清单确实覆盖了两类
// 接入方式，且两者不重叠——同一主机不能既走 ECH 又走不发送 SNI。
func TestRoutedTransportHostListsCoverBothWays(t *testing.T) {
	assert.Contains(t, apiHostnames, "www.pixiv.net")
	assert.Contains(t, apiHostnames, "app-api.pixiv.net")
	assert.Contains(t, imageHostnames, "i.pximg.net")
	for host := range apiHostnames {
		assert.NotContains(t, imageHostnames, host, "主机 %s 不应同时属于两类", host)
	}
}

// TestAutoTransportWiresChannelStates 断言 setup 装配出 API 的 ECH 与不发送 SNI
// 两个自定义通道，且是两份不同的传输。
//
// 通道装配只取决于 base 是否为 *http.Transport；对代理的对待（隐式/显式）由
// ECH 传输自身处理，见 transport_ech.go。
func TestAutoTransportWiresChannelStates(t *testing.T) {
	rt := &AutoTransport{}
	rt.setup()
	_, isECH := rt.ech.(*echTransport)
	require.True(t, isECH, "API 通道应装配 ECH（实际 %T）", rt.ech)
	noSNI, ok := rt.nosni.(*http.Transport)
	require.True(t, ok, "应装配不发送 SNI 的通道（实际 %T）", rt.nosni)
	require.NotNil(t, noSNI.TLSClientConfig)
	assert.Equal(t, noSNIServerName, noSNI.TLSClientConfig.ServerName, "该通道应不发送 SNI")
	assert.NotSame(t, rt.ech, rt.nosni, "两个通道不应共用同一份传输")
}

// TestAutoTransportFallsBackAndVerifiesNoSNIHostname 断言 API 主机的首选方式失败后
// 回退到不发送 SNI 的连接，并在回退响应上补齐主机名校验。
func TestAutoTransportFallsBackAndVerifiesNoSNIHostname(t *testing.T) {
	newRT := func(host string) *AutoTransport {
		var serverName atomic.Value
		base, _ := testTLSServer(t, host, &serverName)
		rt := &AutoTransport{hostNames: &hostSets{api: map[string]struct{}{host: {}}}}
		rt.once.Do(func() {
			rt.base = roundTripperFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("stub: 常规不可用")
			})
			rt.ech = roundTripperFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("stub: ECH 不可用")
			})
			rt.nosni = NewNoSNITransport(base)
		})
		return rt
	}

	t.Run("证书匹配时成功且不发送 SNI", func(t *testing.T) {
		var serverName atomic.Value
		base, rawURL := testTLSServer(t, "example.com", &serverName)
		rt := &AutoTransport{hostNames: &hostSets{api: map[string]struct{}{"example.com": {}}}}
		rt.once.Do(func() {
			rt.base = roundTripperFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("stub: 常规不可用")
			})
			rt.ech = roundTripperFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("stub: ECH 不可用")
			})
			rt.nosni = NewNoSNITransport(base)
		})
		resp, err := (&http.Client{Transport: rt}).Get(rawURL)
		require.NoError(t, err, "ECH 失败后应经不发送 SNI 的方式取回响应")
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "", serverName.Load(), "不发送 SNI 的方式不应发送 SNI")
	})

	t.Run("证书不匹配时失败", func(t *testing.T) {
		var serverName atomic.Value
		_, rawURL := testTLSServer(t, "sni-probe.invalid", &serverName)
		rt := newRT("sni-probe.invalid")
		_, err := (&http.Client{Transport: rt}).Get(rawURL)
		require.Error(t, err, "不发送 SNI 的响应证书与请求主机不匹配时应报错")
		assert.Contains(t, err.Error(), "证书与主机", "错误应说明主机名校验失败")
	})
}

// TestAutoTransportPrefersLastWorkingWay 断言确认某方式失败后，后续请求优先使用
// 上次可用的方式，不再重复无谓的尝试（本实现细节不构成对外契约，
// 见 AutoTransport 的类型文档）。
func TestAutoTransportPrefersLastWorkingWay(t *testing.T) {
	var echCalls int32
	rt := &AutoTransport{hostNames: &hostSets{api: map[string]struct{}{"www.pixiv.net": {}}}}
	rt.once.Do(func() {
		rt.base = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return okResponse(req), nil
		})
		rt.ech = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			atomic.AddInt32(&echCalls, 1)
			return nil, errors.New("stub: ECH 不可用")
		})
		rt.nosni = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return okResponse(req), nil
		})
	})

	req, err := http.NewRequest(http.MethodGet, "https://www.pixiv.net/", nil)
	require.NoError(t, err)
	// 首次：ECH 失败后经不发送 SNI 成功，把它记为可用方式。
	_, err = rt.RoundTrip(req)
	require.NoError(t, err)
	require.Equal(t, int32(1), atomic.LoadInt32(&echCalls), "首次尝试应包括 ECH")

	atomic.StoreInt32(&echCalls, 0)
	// 之后：直接走上次可用的不发送 SNI 方式，不再尝试 ECH。
	_, err = rt.RoundTrip(req)
	require.NoError(t, err)
	assert.Zero(t, atomic.LoadInt32(&echCalls), "已确认 ECH 不可用，后续请求不应再尝试")
}

// TestAutoTransportUsesSingleECHChannel 断言所有 API 主机复用同一份 ECH 通道，
// 图片主机走不发送 SNI 的通道。
//
// ECH 配置由 Cloudflare 全网共享，同一份对任意 Cloudflare 主机都适用，
// 按主机各建一份既无必要（自举会重复发生、配置轮换要各自处理）。
func TestAutoTransportUsesSingleECHChannel(t *testing.T) {
	rt := &AutoTransport{}
	rt.setup()
	ech, isECH := rt.ech.(*echTransport)
	require.True(t, isECH, "应装配 ECH 通道，实际 %T", rt.ech)

	for host := range apiHostnames {
		ways := rt.ways(host)
		require.NotEmpty(t, ways, "主机 %s 应有候选方式", host)
		assert.Equal(t, wayECH, ways[0].id, "主机 %s 首选应为 ECH", host)
		assert.Same(t, ech, ways[0].rt, "主机 %s 应复用同一份 ECH 通道", host)
	}
	for host := range imageHostnames {
		ways := rt.ways(host)
		require.NotEmpty(t, ways, "主机 %s 应有候选方式", host)
		assert.Equal(t, wayNoSNI, ways[0].id, "图片主机 %s 首选应是不发送 SNI", host)
	}
}

// TestNewRoutedTransportAppliesECHToCloudflareHost 断言 API 通道上的 ECH
// 在真实握手下生效。
func TestNewRoutedTransportAppliesECHToCloudflareHost(t *testing.T) {
	server := newECHTestServer(t, true)
	base := server.transport()
	var seen echObserved
	observing(base, &seen)
	injected := newRoutedTransportWithRoutes(
		NewECHTransport(base, WithECHPublicName(echTestPublicName)),
		map[string]route{
			// 注入路由用测试服务端发布的外层名，使外层 SNI 断言可与服务端观测对上。
			echTestRealHost: {rt: NewECHTransport(base, WithECHPublicName(echTestPublicName))},
		},
	)

	resp, err := (&http.Client{Transport: injected}).Get("https://" + echTestRealHost + "/")
	require.NoError(t, err)
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.True(t, seen.accepted, "经路由的请求应真正建立 ECH 连接")
	assert.Equal(t, echTestPublicName, server.seenOuterSNI()[0],
		"服务端在解密前应只看到外层名")
}

// TestRoutedTransportVerifiesImageHostname 断言不发送 SNI 的图片通道由路由层
// 补齐主机名校验：证书与该主机不匹配时请求失败，而不是把响应交给调用者。
//
// 不发送 SNI 后标准库无法自动校验证书主机名，而握手阶段拿不到目标主机名，
// 因此这一步只能发生在路由层——它知道请求主机。
func TestRoutedTransportVerifiesImageHostname(t *testing.T) {
	// 请求主机不在服务端证书的覆盖范围内（证书覆盖 example.com 与 127.0.0.1）。
	var serverName atomic.Value
	base, rawURL := testTLSServer(t, "sni-probe.invalid", &serverName)

	rt := newRoutedTransportWithRoutes(
		NewNoSNITransport(base),
		map[string]route{"sni-probe.invalid": {rt: NewNoSNITransport(base), noSNI: true}},
	)

	_, err := (&http.Client{Transport: rt}).Get(rawURL)
	require.Error(t, err, "证书与请求主机不匹配时应报错")
	assert.Contains(t, err.Error(), "sni-probe.invalid", "错误应指明不匹配的主机")
}

// TestRoutedTransportAcceptsMatchingImageHostname 断言证书与请求主机匹配时
// 图片通道的响应正常返回：上一条用例的断言确实区分了两种行为。
func TestRoutedTransportAcceptsMatchingImageHostname(t *testing.T) {
	var serverName atomic.Value
	base, rawURL := testTLSServer(t, "example.com", &serverName)

	rt := newRoutedTransportWithRoutes(
		NewNoSNITransport(base),
		map[string]route{"example.com": {rt: NewNoSNITransport(base), noSNI: true}},
	)

	resp, err := (&http.Client{Transport: rt}).Get(rawURL)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "", serverName.Load(), "图片通道仍不应发送 SNI")
}

// TestAutoTransportTreatsCallerBaseAsExplicit 断言调用者提供 Base 时，
// 其代理被当作明确意图：ECH 主机因代理无法直连，报错而不是悄悄改走普通连接。
func TestAutoTransportTreatsCallerBaseAsExplicit(t *testing.T) {
	es := newECHTestServer(t, true)
	deadProxy := newECHTestProxy(t, "127.0.0.1:1")
	proxyURL, err := url.Parse(deadProxy.url)
	require.NoError(t, err)

	base := es.transport()
	base.Proxy = http.ProxyURL(proxyURL)
	rt := &AutoTransport{Base: base}

	_, err = (&http.Client{Transport: rt}).Get("https://www.pixiv.net/")
	require.Error(t, err, "显式代理与 ECH 冲突时应报错")
	assert.Contains(t, err.Error(), "ECH")
	assert.Zero(t, deadProxy.connects.Load(), "不应先去尝试代理")
}

// TestAutoTransportTreatsSelfBuiltBaseAsImplicit 断言 Base 未设置（由库自建）时，
// ECH 以隐式代理语义装配：不把继承自环境变量的代理当作明确指令（见
// transport_ech.go），从而 ECH 主机可直连。忽略代理的实际行为由 ECH 传输的
// 测试覆盖，这里只钉 AutoTransport 的装配。
func TestAutoTransportTreatsSelfBuiltBaseAsImplicit(t *testing.T) {
	rt := &AutoTransport{}
	rt.setup()
	ech, ok := rt.ech.(*echTransport)
	require.True(t, ok, "自建 base 时应装配 ECH（实际 %T）", rt.ech)
	assert.True(t, ech.implicitProxy, "自建 base 的 ECH 应视为隐式代理")
}

// TestAutoTransportFallsBackForECHHost 断言 ECH 不可用时自动选择仍能让请求成功：
// 首选失败后会继续尝试其余方式，全部失败时回落到常规连接。
func TestAutoTransportFallsBackForECHHost(t *testing.T) {
	var echCalls, nosniCalls, baseCalls int32
	base := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&baseCalls, 1)
		return okResponse(req), nil
	})
	rt := &AutoTransport{hostNames: &hostSets{api: map[string]struct{}{"www.pixiv.net": {}}}}
	// 让 ECH 与不发送 SNI 方式都必然失败，观察最终回落到常规连接。
	rt.once.Do(func() {
		rt.base = base
		rt.ech = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			atomic.AddInt32(&echCalls, 1)
			return nil, errors.New("stub: ECH 不可用")
		})
		rt.nosni = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			atomic.AddInt32(&nosniCalls, 1)
			return nil, errors.New("stub: 不发送 SNI 不可用")
		})
	})

	req, err := http.NewRequest(http.MethodGet, "https://www.pixiv.net/", nil)
	require.NoError(t, err)
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err, "ECH 不可用时应继续尝试其他方式")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, int32(1), atomic.LoadInt32(&echCalls))
	assert.Equal(t, int32(1), atomic.LoadInt32(&nosniCalls))
	assert.Equal(t, int32(1), atomic.LoadInt32(&baseCalls), "ECH 与不发送 SNI 都失败后应回落到常规连接")
}

// TestAutoTransportSkipsRetryForPlainHost 断言没有特殊方式的主机不触发回落重试。
func TestAutoTransportSkipsRetryForPlainHost(t *testing.T) {
	var baseCalls int32
	base := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&baseCalls, 1)
		return okResponse(req), nil
	})
	rt := &AutoTransport{Base: base}
	req, err := http.NewRequest(http.MethodGet, "https://example.com/", nil)
	require.NoError(t, err)
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, int32(1), atomic.LoadInt32(&baseCalls), "无特殊方式的主机不应重试")
}

// TestNewRoutedTransportWithPlainRoundTripper 断言两个通道都不是 *http.Transport
// 时公开入口仍可用，请求照常发出。
func TestNewRoutedTransportWithPlainRoundTripper(t *testing.T) {
	var count int32
	plain := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&count, 1)
		return okResponse(req), nil
	})
	rt := NewRoutedTransport(plain, plain)
	req, err := http.NewRequest(http.MethodGet, "https://i.pximg.net/x.png", nil)
	require.NoError(t, err)
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, int32(1), atomic.LoadInt32(&count))
}

// testTLSServer 启动一个本地 TLS 服务端，并返回一个把指定主机名拨到该
// 服务端的传输。请求因此使用真实主机名（而非 IP 字面量），TLS 的 SNI
// 行为才与真实场景一致——IP 字面量本身就不会产生 SNI 扩展。
//
// 请求主机名由 host 指定：服务端的自签证书覆盖 example.com，因此需要
// 校验通过的用例传该名字，需要观察「证书与主机不匹配」的用例传其他名字。
func testTLSServer(t *testing.T, host string, serverName *atomic.Value) (base *http.Transport, rawURL string) {
	t.Helper()
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server.TLS = &tls.Config{
		GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
			serverName.Store(hello.ServerName)
			return nil, nil
		},
	}
	server.StartTLS()
	t.Cleanup(server.Close)

	var addr = server.Listener.Addr().String()
	base = defaultBaseTransport()
	// 拨号被重定向到本地测试服务端，代领会破坏这一劫持，因此显式禁用，
	// 使测试不受 HTTPS_PROXY 等代理环境变量影响。
	base.Proxy = nil
	base.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, addr)
	}
	// 只关心 SNI 与证书主机名的处理，因此跳过标准库的证书校验：本地服务端的
	// 自签证书不在系统根证书里，否则校验会先于本次断言的目标失败。
	// 证书链与主机名由 NewNoSNITransport 叠加的校验负责（见 transport_no_sni.go）。
	base.TLSClientConfig.InsecureSkipVerify = true
	base.TLSClientConfig.RootCAs = x509.NewCertPool()
	base.TLSClientConfig.RootCAs.AddCert(server.Certificate())
	return base, "https://" + host + "/"
}

// TestTransportPrimitivesCompose 断言原语嵌套后请求按预期经过各层：
// 经两层无 SNI 原语发出的请求仍能完成握手，且服务端观测不到 SNI。
func TestTransportPrimitivesCompose(t *testing.T) {
	var serverName atomic.Value
	base, rawURL := testTLSServer(t, "example.com", &serverName)

	client := &http.Client{Transport: NewNoSNITransport(NewNoSNITransport(base))}
	resp, err := client.Get(rawURL)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "", serverName.Load(), "嵌套组合后仍不应发送 SNI")
}

// TestNewNoSNITransportSuppressesSNI 断言原语确实不发送 SNI：
// 本地 TLS 服务端能够观测到客户端未提供 SNI。
func TestNewNoSNITransportSuppressesSNI(t *testing.T) {
	var serverName atomic.Value
	base, rawURL := testTLSServer(t, "example.com", &serverName)

	client := &http.Client{Transport: NewNoSNITransport(base)}
	resp, err := client.Get(rawURL)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "", serverName.Load(), "不发送 SNI 时服务端不应收到 ServerName")
}

// TestDefaultTransportBaselineSendsSNI 断言对照组（未经原语）会发送 SNI，
// 以确认上一个用例的断言确实区分了两种行为。
func TestDefaultTransportBaselineSendsSNI(t *testing.T) {
	var serverName atomic.Value
	base, rawURL := testTLSServer(t, "example.com", &serverName)

	client := &http.Client{Transport: base}
	resp, err := client.Get(rawURL)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotEqual(t, "", serverName.Load(), "未经原语的请求应发送 SNI")
}

// TestNewNoSNITransportDoesNotMutateBase 断言原语克隆基础传输，
// 因此可与其他原语嵌套组合且不改变调用者的传输。
func TestNewNoSNITransportDoesNotMutateBase(t *testing.T) {
	base := defaultBaseTransport()
	base.TLSClientConfig.VerifyPeerCertificate = func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
		return nil
	}
	baseDial := base.DialContext
	baseVerify := base.TLSClientConfig.VerifyPeerCertificate

	rt := NewNoSNITransport(base)

	assert.NotSame(t, base, rt, "应返回克隆而非改动调用者的传输")
	assert.Equal(t, "", base.TLSClientConfig.ServerName, "不应改变调用者基础传输的 ServerName")
	assert.False(t, base.TLSClientConfig.InsecureSkipVerify, "不应改变调用者基础传输的校验开关")
	assert.NotNil(t, baseVerify, "不应移除调用者基础传输的校验回调")
	assert.NotNil(t, baseDial, "不应移除调用者基础传输的拨号函数")

	require.NotNil(t, rt.TLSClientConfig)
	assert.Equal(t, noSNIServerName, rt.TLSClientConfig.ServerName)

	// 可嵌套：在已叠加的传输之上再叠加一次仍然可用且各自独立。
	again := NewNoSNITransport(rt)
	require.NotNil(t, again.TLSClientConfig)
	assert.Equal(t, noSNIServerName, again.TLSClientConfig.ServerName)
}

// TestNewNoSNITransportKeepsCallerVerification 断言调用者原有的证书校验回调
// 仍然生效：原语叠加能力而非替换调用者的要求。
func TestNewNoSNITransportKeepsCallerVerification(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(server.Close)
	var cert = server.Certificate()
	var pool = x509.NewCertPool()
	pool.AddCert(cert)

	base := defaultBaseTransport()
	base.TLSClientConfig.RootCAs = pool
	var called bool
	base.TLSClientConfig.VerifyPeerCertificate = func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
		called = true
		return errors.New("stub: 调用者的校验拒绝")
	}

	rt := NewNoSNITransport(base)
	require.NotNil(t, rt.TLSClientConfig.VerifyPeerCertificate)

	// 证书链本身可信（已放入 RootCAs），因此叠加的校验通过，
	// 随后调用者的回调被执行并拒绝。
	err := rt.TLSClientConfig.VerifyPeerCertificate([][]byte{cert.Raw}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stub: 调用者的校验拒绝")
	assert.True(t, called, "调用者的校验回调应被执行")
}

// TestNewNoSNITransportDoesNotUseDialTLS 断言原语不用 DialTLSContext：
// 该手段在经代理的请求上会被静默忽略，能力不生效。
func TestNewNoSNITransportDoesNotUseDialTLS(t *testing.T) {
	rt := NewNoSNITransport(defaultBaseTransport())
	assert.Nil(t, rt.DialTLSContext)
	assert.Nil(t, rt.DialTLS)
	require.NotNil(t, rt.TLSClientConfig, "TLS 层能力应由 TLSClientConfig 施加")
}

// TestNewNoSNITransportNilBase 断言未提供基础传输时使用进程默认传输。
func TestNewNoSNITransportNilBase(t *testing.T) {
	rt := NewNoSNITransport(nil)
	require.NotNil(t, rt)
	require.NotNil(t, rt.TLSClientConfig)
	assert.Equal(t, noSNIServerName, rt.TLSClientConfig.ServerName)
}

// TestNewNoSNITransportVerifiesCertificateChain 断言不发送 SNI 时
// 仍会校验证书链，而不是无条件信任服务端。
func TestNewNoSNITransportVerifiesCertificateChain(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	// 用系统根证书校验自签证书，应当失败。
	rt := NewNoSNITransport(defaultBaseTransport())
	client := &http.Client{Transport: rt}
	_, err := client.Get(server.URL)
	require.Error(t, err, "自签证书不应通过校验")

	// 放入该服务端的根证书后应能通过，说明校验确实按证书链进行。
	// 主机名不在这里校验（原语不知道目标主机名），由路由层在收到响应后补齐，
	// 见 TestRoutedTransportVerifiesImageHostname。
	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	rt = NewNoSNITransport(defaultBaseTransport())
	rt.TLSClientConfig.RootCAs = pool
	rt.TLSClientConfig.VerifyPeerCertificate = verifyPeerCertificate(pool)
	client = &http.Client{Transport: rt}
	resp, err := client.Get(server.URL)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestAutoTransportAggregatesErrorsWhenAllWaysFail 断言自动选择在全部方式
// 都不可用时把每种方式的失败原因一起返回给调用者。
func TestAutoTransportAggregatesErrorsWhenAllWaysFail(t *testing.T) {
	rt := &AutoTransport{hostNames: &hostSets{image: map[string]struct{}{"i.pximg.net": {}}}}
	rt.once.Do(func() {
		rt.base = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("stub: 常规不可用")
		})
		rt.nosni = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("stub: 不发送 SNI 不可用")
		})
	})
	req, err := http.NewRequest(http.MethodGet, "https://i.pximg.net/x.png", nil)
	require.NoError(t, err)
	_, err = rt.RoundTrip(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stub: 不发送 SNI 不可用")
	assert.Contains(t, err.Error(), "stub: 常规不可用")
}

// TestAutoTransportSucceeds 断言自动选择最终能让请求成功；
// 不断言它用了哪种方式，因为那不构成对外承诺。
func TestAutoTransportSucceeds(t *testing.T) {
	rt := &AutoTransport{
		Base: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return okResponse(req), nil
		}),
	}
	req, err := http.NewRequest(http.MethodGet, "https://example.com/", nil)
	require.NoError(t, err)
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestAutoTransportContinuesToOtherWays 断言某个方式不可用时自动选择
// 仍能让请求成功。只断言外部可观测结果，不断言它用了哪种方式。
func TestAutoTransportContinuesToOtherWays(t *testing.T) {
	base := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return okResponse(req), nil
	})
	rt := &AutoTransport{hostNames: &hostSets{image: map[string]struct{}{"i.pximg.net": {}}}}
	// 让图片主机的首选方式（不发送 SNI）必然失败，观察回落到常规连接。
	rt.once.Do(func() {
		rt.base = base
		rt.nosni = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("stub: 不发送 SNI 不可用")
		})
	})

	req, err := http.NewRequest(http.MethodGet, "https://i.pximg.net/x.png", nil)
	require.NoError(t, err)
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err, "某个方式不可用时应继续尝试其余方式")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
