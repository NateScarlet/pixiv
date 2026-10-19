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
	"sync"
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

// TestNewNoSNITransportReturnsOpaqueRoundTripper 断言原语的返回值是不透明类型：
// 调用者只依赖 http.RoundTripper，库不暴露底层 *http.Transport。
func TestNewNoSNITransportReturnsOpaqueRoundTripper(t *testing.T) {
	var rt http.RoundTripper = NewNoSNITransport(defaultBaseTransport())
	require.NotNil(t, rt)
	_, isPlain := rt.(*http.Transport)
	assert.False(t, isPlain, "不应把底层传输暴露给调用者")
}

// TestNoSNITransportReleasesIdleConnections 断言调用者经 http.Client
// 释放空闲连接对本原语仍然有效：它不再暴露底层传输，调用者唯一的释放途径
// 就是标准库的 http.Client.CloseIdleConnections。
func TestNoSNITransportReleasesIdleConnections(t *testing.T) {
	var serverName atomic.Value
	var server = newTLSServer(t, statusHandler(http.StatusOK), &serverName)
	var base = dialTo(server)
	var dials int32
	var dial = base.DialContext
	base.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		atomic.AddInt32(&dials, 1)
		return dial(ctx, network, addr)
	}

	client := &http.Client{Transport: NewNoSNITransport(base)}
	var get = func() {
		t.Helper()
		resp, err := client.Get(server.URL)
		require.NoError(t, err)
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	get()
	get()
	require.Equal(t, int32(1), atomic.LoadInt32(&dials), "第二次请求应复用连接")

	client.CloseIdleConnections()
	get()
	assert.Equal(t, int32(2), atomic.LoadInt32(&dials), "释放空闲连接后应重新拨号")
}

// TestAutoTransportWiresWorkingChannels 断言 setup 装配出的两个自定义通道确实可用：
// 不发送 SNI 的通道经它发出的请求完成握手且服务端观测不到 SNI。
//
// 通道装配只取决于 base 是否为 *http.Transport；对代理的对待（隐式/显式）由
// ECH 传输自身处理，见 transport_ech.go。
func TestAutoTransportWiresWorkingChannels(t *testing.T) {
	var serverName atomic.Value
	var server = newTLSServer(t, statusHandler(http.StatusOK), &serverName)

	rt := &AutoTransport{
		Base:      dialTo(server),
		hostNames: &hostSets{image: map[string]struct{}{"example.com": {}}},
	}
	rt.setup()
	require.NotNil(t, rt.ech)
	require.NotNil(t, rt.nosni)
	assert.NotSame(t, rt.ech, rt.nosni, "两个通道不应共用同一份传输")

	resp, err := (&http.Client{Transport: rt}).Get("https://example.com/x.png")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "", serverName.Load(), "装配出的通道应不发送 SNI")
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

// TestAutoTransportSkipsRejectedNoSNITransport 断言服务端拒绝不发送 SNI 的传输之后，
// 自动选择既不会把该传输的拒绝当成可用、也不会把它留在后续请求的首选：
// 这正是「ECH 抖动一次、此后全部 403」的成因。
func TestAutoTransportSkipsRejectedNoSNITransport(t *testing.T) {
	const host = "api.example.com"
	var serverName atomic.Value
	var rejected int32
	var rejecting = newTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&rejected, 1)
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("<html>403 Forbidden</html>"))
	}), &serverName)

	rt := &AutoTransport{hostNames: &hostSets{api: map[string]struct{}{host: {}}}}
	rt.once.Do(func() {
		rt.ech = roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("stub: ECH 不可用")
		})
		// 真实的 noSNI 传输：拨到以 403 一律拒绝的本地服务端。
		rt.nosni = NewNoSNITransport(dialTo(rejecting))
		rt.base = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return okResponse(req), nil
		})
	})

	client := &http.Client{Transport: rt}
	for i := range 2 {
		resp, err := client.Get("https://" + host + "/ajax/search/novels/x")
		require.NoError(t, err, "第 %d 次请求：被拒绝的传输应让路给其余方式", i+1)
		resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	}
	assert.Equal(t, int32(1), atomic.LoadInt32(&rejected),
		"被拒绝的传输只应在首次请求中被尝试一次，后续请求不应再复用它")
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
		var candidates = rt.ways(host)
		require.NotEmpty(t, candidates, "主机 %s 应有候选方式", host)
		assert.Equal(t, []way{wayECH, wayNoSNI, wayPlain}, wayIDs(candidates), "主机 %s 的候选顺序", host)
		assert.Same(t, ech, candidates[0].rt, "主机 %s 应复用同一份 ECH 通道", host)
	}
	for host := range imageHostnames {
		var candidates = rt.ways(host)
		require.NotEmpty(t, candidates, "主机 %s 应有候选方式", host)
		assert.Equal(t, []way{wayNoSNI, wayPlain}, wayIDs(candidates), "图片主机 %s 的候选顺序", host)
	}
}

// wayIDs 取出候选方式的标识，供断言候选顺序。
func wayIDs(ways []candidate) []way {
	var ids = make([]way, len(ways))
	for i, c := range ways {
		ids[i] = c.id
	}
	return ids
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
	base, rawURL = testTLSServerWithStatus(t, host, http.StatusOK, serverName)
	return base, rawURL
}

// testTLSServerWithStatus 与 [testTLSServer] 相同，但让服务端以指定状态码应答，
// 用于观测原语对不同状态的处理。
func testTLSServerWithStatus(
	t *testing.T, host string, status int, serverName *atomic.Value,
) (base *http.Transport, rawURL string) {
	t.Helper()
	var server = newTLSServer(t, statusHandler(status), serverName)
	base = dialTo(server)
	base.TLSClientConfig.RootCAs.AddCert(server.Certificate())
	return base, "https://" + host + "/"
}

// dialTo 返回把任意目标都拨到给定服务端的传输，使请求无论带什么主机名都能抵达
// 该服务端。只关心 SNI 与证书主机名的处理，因此跳过标准库的证书校验：本地服务端的
// 自签证书不在系统根证书里，否则校验会先于本次断言的目标失败。
// 证书链与主机名由叠加其上的原语负责（见 transport_no_sni.go）。
func dialTo(server *httptest.Server) *http.Transport {
	base := defaultBaseTransport()
	// 拨号被重定向到本地测试服务端，代理会破坏这一劫持，因此显式禁用，
	// 使测试不受 HTTPS_PROXY 等代理环境变量影响。
	base.Proxy = nil
	base.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, server.Listener.Addr().String())
	}
	base.TLSClientConfig.InsecureSkipVerify = true
	base.TLSClientConfig.RootCAs = x509.NewCertPool()
	return base
}

// newTLSServer 启动一个本地 TLS 服务端，记录握手收到的 SNI 并以 handler 应答。
// 服务端监听在真实地址上，因此需要经代理抵达它的用例可以直接转发到它。
func newTLSServer(t *testing.T, handler http.Handler, serverName *atomic.Value) *httptest.Server {
	t.Helper()
	var server = httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{
		GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
			serverName.Store(hello.ServerName)
			return nil, nil
		},
	}
	server.StartTLS()
	t.Cleanup(server.Close)
	return server
}

// statusHandler 以指定状态码应答。应答带非空正文：观测「响应体是否被关闭」
// 需要非空的正文。
func statusHandler(status int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		w.Write([]byte("pixiv"))
	})
}

// TestTransportPrimitivesCompose 断言原语嵌套组合后请求按预期经过各层：
// 经「目标别名 + 不发送 SNI」两层原语发出的请求仍能完成握手，且服务端观测不到 SNI。
func TestTransportPrimitivesCompose(t *testing.T) {
	var serverName atomic.Value
	base, rawURL := testTLSServer(t, "example.com", &serverName)

	client := &http.Client{Transport: NewNoSNITransport(NewHostAliasTransport(base, nil))}
	resp, err := client.Get(rawURL)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "", serverName.Load(), "嵌套组合后仍不应发送 SNI")
}

// TestNoSNITransportReportsForbiddenAsError 断言服务端以 403 拒绝时原语报错，
// 而不是把该响应交给调用者：不发送 SNI 的握手不含主机名，服务端只能落到默认
// server block，它给出的 403 是该传输对该主机不可用的证据，不是应答。
func TestNoSNITransportReportsForbiddenAsError(t *testing.T) {
	var serverName atomic.Value
	base, rawURL := testTLSServerWithStatus(t, "example.com", http.StatusForbidden, &serverName)

	client := &http.Client{Transport: NewNoSNITransport(base)}
	_, err := client.Get(rawURL)
	require.Error(t, err, "403 应报为错误而不是把响应交给调用者")

	var rej *ErrAPIRejected
	require.ErrorAs(t, err, &rej, "错误应可取回被拒响应的状态")
	require.NotNil(t, rej.Response)
	assert.Equal(t, http.StatusForbidden, rej.Response.StatusCode, "状态信息应可读")
	// 响应已不可用，其正文必须已关闭，否则连接与内存都泄漏给一个没人会消费的正文。
	n, readErr := rej.Response.Body.Read(make([]byte, 8))
	assert.Zero(t, n, "转成错误时响应体必须关闭")
	assert.ErrorContains(t, readErr, "closed")
}

// TestNoSNITransportPassesThroughOtherStatuses 断言 403 之外的响应原样返回：
// 扩大报错范围需要有对应的实测依据，业务状态码（404、429、503）不是传输不可用的证据。
func TestNoSNITransportPassesThroughOtherStatuses(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusNotFound, http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var serverName atomic.Value
			base, rawURL := testTLSServerWithStatus(t, "example.com", status, &serverName)

			client := &http.Client{Transport: NewNoSNITransport(base)}
			resp, err := client.Get(rawURL)
			require.NoError(t, err, "403 之外的响应不应报错")
			defer resp.Body.Close()
			assert.Equal(t, status, resp.StatusCode)
			assert.Equal(t, "", serverName.Load(), "不发送 SNI 的能力不受影响")
		})
	}
}

// #region 需要源站别名的不发送 SNI 的通道

// aliasProbeHost 与 aliasProbeOrigin 是别名探测用的请求主机与其源站别名。
//
// 都不需要真实存在：别名与代理的冲突与主机是否真实无关，用例用拨号劫持或本地代理
// 把请求引到本地服务端。
const (
	aliasProbeHost   = "alias-probe.invalid"
	aliasProbeOrigin = "origin-probe.invalid"
	// imageProbeHost 代表不需要别名的主机（图片主机这一类）：直连与经代理落到同一对端。
	imageProbeHost = "image-probe.invalid"
)

// TestNoSNIAliasIgnoresImplicitProxy 断言隐式代理（base 由本库自建、代理只来自
// 进程环境变量）下，需要源站别名的主机仍直达源站。
//
// 别名在拨号时才施加（见 [NewHostAliasTransport]），因此只在直连时生效：经代理时
// 拨号目标是代理地址，别名分支不进入，请求会被代理解析到别名之外的对端，这条通道
// 就此失去它存在的意义。
func TestNoSNIAliasIgnoresImplicitProxy(t *testing.T) {
	var serverName atomic.Value
	base, rawURL := testTLSServer(t, aliasProbeHost, &serverName)
	// 代理指向必然失败的地址：若请求真的经它发出，用例会失败。
	base.Proxy = http.ProxyURL(deadProxyURL(t))

	var mu sync.Mutex
	var resolved []string
	// implicit=true：等价于 AutoTransport 自建 base 的情形。
	rt := newAliasedNoSNITransport(base, true, map[string]string{aliasProbeHost: aliasProbeOrigin})
	c := New(
		WithTransport(rt),
		WithDNSResolver(resolverFunc(func(_ context.Context, host string) ([]net.IP, error) {
			mu.Lock()
			resolved = append(resolved, host)
			mu.Unlock()
			return []net.IP{net.ParseIP("127.0.0.1")}, nil
		})),
	)

	resp, err := c.Get(rawURL)
	require.NoError(t, err, "隐式代理应被忽略，请求直达源站")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "", serverName.Load(), "不发送 SNI 的能力不受影响")

	mu.Lock()
	defer mu.Unlock()
	assert.Contains(t, resolved, aliasProbeOrigin, "别名应重新生效：解析的是源站")
}

// TestNewNoSNIAliasTransportNilBaseIsImplicit 断言导出的构造把「未提供 base」
// 映射为运行时的隐式代理语义（与 [NewECHTransport] 的 nil 处理一致）。
//
// 隐式代理的实际行为在内部接缝上验证（TestNoSNIAliasIgnoresImplicitProxy）：
// 这里无法注入环境代理——默认 base 的 Proxy 来自 ProxyFromEnvironment，而它
// 有进程级缓存，不适合作为测试前提。
func TestNewNoSNIAliasTransportNilBaseIsImplicit(t *testing.T) {
	rt := NewNoSNIAliasTransport(nil)
	alias, ok := rt.(*aliasedNoSNITransport)
	require.True(t, ok, "应返回该通道的实现（实际 %T）", rt)
	assert.True(t, alias.implicitProxy, "未提供 base 时应按库自建处理")

	req, err := http.NewRequest(http.MethodGet, "https://www.pixiv.net/", nil)
	require.NoError(t, err)
	assert.False(t, alias.usesProxy(req), "隐式代理下需要别名的主机不应经代理")
}

// TestNewNoSNIAliasTransportMatchesRuntime 断言导出的构造与 AutoTransport 装配出的
// no-SNI 候选是同一份语义：调用者提供带代理的 base 时，两者对需要别名的主机给出
// 同样的错误，且都不先连代理。
//
// 诊断工具经该入口复现运行时通道，因此这条等价性是它「总是体现当前实现」的依据。
func TestNewNoSNIAliasTransportMatchesRuntime(t *testing.T) {
	base := defaultBaseTransport()
	base.Proxy = http.ProxyURL(deadProxyURL(t))

	explicit := NewNoSNIAliasTransport(base)
	rt := &AutoTransport{Base: base}
	rt.setup()

	req, err := http.NewRequest(http.MethodGet, "https://www.pixiv.net/", nil)
	require.NoError(t, err)
	_, explicitErr := explicit.RoundTrip(req)
	_, runtimeErr := rt.nosni.RoundTrip(req.Clone(req.Context()))

	require.Error(t, explicitErr, "调用者的代理与源站别名冲突时应报错")
	require.Error(t, runtimeErr, "运行时通道应给出同样的结论")
	assert.Contains(t, explicitErr.Error(), "源站别名")
	assert.Equal(t, runtimeErr.Error(), explicitErr.Error(),
		"探测用的构造应与运行时通道给出相同结论")
}

// TestNewNoSNIAliasTransportKeepsCallerProxyExplicit 断言调用者传入 base 时，
// 其中的代理是显式指令：需要别名的主机报错而不是静默经代理发出。
func TestNewNoSNIAliasTransportKeepsCallerProxyExplicit(t *testing.T) {
	deadProxy := newECHTestProxy(t, "127.0.0.1:1")
	base := defaultBaseTransport()
	base.Proxy = http.ProxyURL(mustProxyURL(t, deadProxy.url))

	_, err := (&http.Client{Transport: NewNoSNIAliasTransport(base)}).Get("https://www.pixiv.net/")
	require.Error(t, err, "调用者提供的代理与源站别名冲突时应报错")
	assert.Contains(t, err.Error(), "源站别名")
	assert.Zero(t, deadProxy.connects.Load(), "报错应发生在发起连接之前")
}

// TestNoSNIAliasErrorsOnExplicitProxy 断言调用者显式指定代理时快速失败：
// 别名在代理下无法生效，请求会落到别名之外的对端，此时应报错说明冲突，
// 而不是发出一次注定打到错误对端的请求。
func TestNoSNIAliasErrorsOnExplicitProxy(t *testing.T) {
	deadProxy := newECHTestProxy(t, "127.0.0.1:1")
	base := defaultBaseTransport()
	base.Proxy = http.ProxyURL(mustProxyURL(t, deadProxy.url))

	rt := newAliasedNoSNITransport(base, false, map[string]string{aliasProbeHost: aliasProbeOrigin})
	_, err := (&http.Client{Transport: rt}).Get("https://" + aliasProbeHost + "/")
	require.Error(t, err, "显式代理与源站别名冲突时应报错")
	assert.Contains(t, err.Error(), "源站别名")
	assert.Contains(t, err.Error(), "代理")
	assert.Zero(t, deadProxy.connects.Load(), "报错应发生在发起连接之前")
}

// TestNoSNIAliasKeepsProxyForOtherHosts 断言本层只处理需要别名的主机：
// 其余主机（例如图片主机）经代理时行为不变——仍经代理、仍不发送 SNI。
func TestNoSNIAliasKeepsProxyForOtherHosts(t *testing.T) {
	var serverName atomic.Value
	server := newTLSServer(t, statusHandler(http.StatusOK), &serverName)
	proxy := newECHTestProxy(t, server.Listener.Addr().String())

	base := defaultBaseTransport()
	base.TLSClientConfig.InsecureSkipVerify = true
	base.Proxy = http.ProxyURL(mustProxyURL(t, proxy.url))

	rt := newAliasedNoSNITransport(base, false, map[string]string{aliasProbeHost: aliasProbeOrigin})
	resp, err := (&http.Client{Transport: rt}).Get("https://" + imageProbeHost + "/")
	require.NoError(t, err, "不需要别名的主机不受本层处理影响")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "", serverName.Load(), "经代理时仍不发送 SNI")
}

// TestNoSNIAliasKeepsCallerProxySettings 断言两种代理来源下都不改变调用者传输的
// 代理设置：忽略隐式代理只发生在该通道自己的克隆上。
func TestNoSNIAliasKeepsCallerProxySettings(t *testing.T) {
	base := defaultBaseTransport()
	proxyURL := mustProxyURL(t, "http://127.0.0.1:7890")
	base.Proxy = http.ProxyURL(proxyURL)

	for _, implicit := range []bool{false, true} {
		_ = newAliasedNoSNITransport(base, implicit, map[string]string{aliasProbeHost: aliasProbeOrigin})

		require.NotNil(t, base.Proxy, "不应清空调用者传输的代理设置（implicit=%v）", implicit)
		req, err := http.NewRequest(http.MethodGet, "https://"+aliasProbeHost+"/", nil)
		require.NoError(t, err)
		got, err := base.Proxy(req)
		require.NoError(t, err)
		assert.Equal(t, proxyURL.String(), got.String(),
			"调用者的传输仍应使用代理（implicit=%v）", implicit)
	}
}

// TestAutoTransportPassesProxySourceToAliasChannel 断言装配把代理的来源
// （隐式/显式）传给需要别名的主机的不发送 SNI 的通道，与 ECH 同一套语义。
func TestAutoTransportPassesProxySourceToAliasChannel(t *testing.T) {
	selfBuilt := &AutoTransport{}
	selfBuilt.setup()
	selfBuiltAlias, ok := selfBuilt.nosni.(*aliasedNoSNITransport)
	require.True(t, ok, "自建 base 时应装配该通道（实际 %T）", selfBuilt.nosni)
	assert.True(t, selfBuiltAlias.implicitProxy, "自建 base 的代理应视为隐式")

	callerBase := &AutoTransport{Base: defaultBaseTransport()}
	callerBase.setup()
	callerAlias, ok := callerBase.nosni.(*aliasedNoSNITransport)
	require.True(t, ok, "调用者提供 base 时同样装配该通道（实际 %T）", callerBase.nosni)
	assert.False(t, callerAlias.implicitProxy, "调用者的 base 中的代理应视为显式")
}

// TestAutoTransportAliasNeedsDirectConnection 断言需要源站别名的主机不会把请求
// 发给调用者显式指定的代理：别名在代理下无法生效，请求会落到别名之外的对端。
//
// 该候选报错不应让整次请求失败：AutoTransport 仍经常规通道取得响应。
func TestAutoTransportAliasNeedsDirectConnection(t *testing.T) {
	var serverName atomic.Value
	server := newTLSServer(t, statusHandler(http.StatusOK), &serverName)
	proxy := newECHTestProxy(t, server.Listener.Addr().String())

	base := defaultBaseTransport()
	base.TLSClientConfig.InsecureSkipVerify = true
	base.Proxy = http.ProxyURL(mustProxyURL(t, proxy.url))

	// 需要别名的主机取库的清单里的真实主机：别名是库的知识（见 noSNIHostTarget）。
	rt := &AutoTransport{
		Base:      base,
		hostNames: &hostSets{api: map[string]struct{}{"www.pixiv.net": {}}},
	}
	resp, err := (&http.Client{Transport: rt}).Get("https://www.pixiv.net/")
	require.NoError(t, err, "该候选报错不应让整次请求失败")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, int64(1), proxy.connects.Load(),
		"只有常规通道把请求发给代理；需要别名的主机不应发出这次请求")
}

// #endregion

// deadProxyURL 返回指向必然失败地址的代理 URL：需要「请求不经代理」的用例用它，
// 使「真的走了代理」表现为请求失败，而不是被静默忽略。
func deadProxyURL(t *testing.T) *url.URL {
	t.Helper()
	return mustProxyURL(t, "http://127.0.0.1:1")
}

// mustProxyURL 解析代理 URL。
func mustProxyURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u
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
//
// 在接缝上表达：不发送 SNI 的能力只出现在原语上，调用者的 base 仍照常发送 SNI。
func TestNewNoSNITransportDoesNotMutateBase(t *testing.T) {
	var serverName atomic.Value
	base, rawURL := testTLSServer(t, "example.com", &serverName)

	resp, err := (&http.Client{Transport: NewNoSNITransport(base)}).Get(rawURL)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, "", serverName.Load(), "原语应返回克隆而非改动调用者的传输")

	resp, err = (&http.Client{Transport: base}).Get(rawURL)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.NotEqual(t, "", serverName.Load(), "调用者的传输应保持原样，仍照常发送 SNI")
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

	client := &http.Client{Transport: NewNoSNITransport(base)}
	// 证书链本身可信（已放入 RootCAs），因此叠加的校验通过，
	// 随后调用者的回调被执行并拒绝——经一次真实握手在接缝上表达。
	_, err := client.Get(server.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stub: 调用者的校验拒绝")
	assert.True(t, called, "调用者的校验回调应被执行")
}

// TestNewNoSNITransportKeepsCapabilityBehindProxy 断言经代理时
// 不发送 SNI 的能力仍然生效：它由 TLS 客户端配置承载，而非 DialTLSContext
// ——标准库明确后者只对 non-proxied 请求生效，存在代理时会被静默忽略。
func TestNewNoSNITransportKeepsCapabilityBehindProxy(t *testing.T) {
	// 走真实地址而不是 testTLSServer 的拨号劫持：经代理时客户端先拨代理，
	// 劫持会把 CONNECT 请求直接送到 TLS 服务端。
	var serverName atomic.Value
	var server = newTLSServer(t, statusHandler(http.StatusOK), &serverName)
	proxyURL, err := url.Parse(newECHTestProxy(t, server.Listener.Addr().String()).url)
	require.NoError(t, err)

	base := defaultBaseTransport()
	base.TLSClientConfig.InsecureSkipVerify = true
	base.Proxy = http.ProxyURL(proxyURL)

	client := &http.Client{Transport: NewNoSNITransport(base)}
	resp, err := client.Get(server.URL)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "", serverName.Load(), "经代理时仍不应发送 SNI")
}

// TestNewNoSNITransportNilBase 断言未提供基础传输时使用进程默认传输。
func TestNewNoSNITransportNilBase(t *testing.T) {
	// 进程默认传输没有把请求劫持到本地，因此这里只断言原语可用：
	// base 为空时不得返回空 RoundTripper 或 panic。
	var rt http.RoundTripper = NewNoSNITransport(nil)
	assert.NotNil(t, rt)
}

// TestNewNoSNITransportVerifiesCertificateChain 断言不发送 SNI 时
// 仍会校验证书链，而不是无条件信任服务端。
func TestNewNoSNITransportVerifiesCertificateChain(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	// 用系统根证书校验自签证书，应当失败。
	client := &http.Client{Transport: NewNoSNITransport(defaultBaseTransport())}
	_, err := client.Get(server.URL)
	require.Error(t, err, "自签证书不应通过校验")

	// 在 base 上放入该服务端的根证书后应能通过，说明校验确实按证书链进行。
	// 主机名不在这里校验（原语不知道目标主机名），由路由层在收到响应后补齐，
	// 见 TestRoutedTransportVerifiesImageHostname。
	base := defaultBaseTransport()
	base.TLSClientConfig.RootCAs = x509.NewCertPool()
	base.TLSClientConfig.RootCAs.AddCert(server.Certificate())
	client = &http.Client{Transport: NewNoSNITransport(base)}
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
