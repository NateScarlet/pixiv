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

// newRoutedTransportWithRoutes 是测试专用的路由构造，用于注入任意主机清单，
// 从而可以在断言路由行为时不断言库持有的真实清单内容。
func newRoutedTransportWithRoutes(base http.RoundTripper, routes map[string]http.RoundTripper) *routedTransport {
	return &routedTransport{base: base, routes: routes}
}

// TestRoutedTransportSendsHostToMatchingTransport 断言请求到达适合该主机的
// 传输，其他主机仍走基础传输。
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
	rt := newRoutedTransportWithRoutes(baseRT, map[string]http.RoundTripper{
		"special.example.com": specialRT,
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
	assert.Equal(t, int32(1), atomic.LoadInt32(&special), "其他主机不应触发特殊传输")
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
		map[string]http.RoundTripper{"special.example.com": redirect},
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
		map[string]http.RoundTripper{"special.example.com": redirect},
	)
	req, err := http.NewRequest(http.MethodGet, "https://special.example.com/a", nil)
	require.NoError(t, err)
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, "https://special.example.com/next", resp.Header.Get("Location"))
}

// TestNewRoutedTransportAppliesECHToCloudflareHost 断言路由到清单内主机的传输
// 确实施加 ECH，且该能力在真实握手下生效。
//
// 分两步：结构上断言清单内的主机被路由到 ECH 传输；行为上用一个与清单等价的
// 注入路由，把请求指向本地 ECH 服务端，断言 ECH 被真正接受。
func TestNewRoutedTransportAppliesECHToCloudflareHost(t *testing.T) {
	// 结构与库持有的真实清单一致：清单内的主机各有专门的传输。
	rt := NewRoutedTransport(defaultBaseTransport())
	rs, ok := rt.(*routedTransport)
	require.True(t, ok)
	for host := range echHostnames {
		route, routed := rs.routes[host]
		require.True(t, routed, "主机 %s 应有专门的传输", host)
		_, isECH := route.(*echTransport)
		assert.True(t, isECH, "主机 %s 应被路由到 ECH 传输", host)
	}

	// 行为：同样的路由方式下 ECH 真正生效。
	server := newECHTestServer(t, true)
	base := server.transport()
	var seen echObserved
	observing(base, &seen)
	injected := newRoutedTransportWithRoutes(base, map[string]http.RoundTripper{
		// 注入路由用测试服务端发布的外层名，使外层 SNI 断言可与服务端观测对上。
		echTestRealHost: NewECHTransport(base, WithECHPublicName(echTestPublicName)),
	})

	resp, err := (&http.Client{Transport: injected}).Get("https://" + echTestRealHost + "/")
	require.NoError(t, err)
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.True(t, seen.accepted, "经路由的请求应真正建立 ECH 连接")
	assert.Equal(t, echTestPublicName, server.seenOuterSNI()[0],
		"服务端在解密前应只看到外层名")
}

// TestNewRoutedTransportSkipsECHForInapplicableHosts 断言不适用 ECH 的主机不被
// 施加 ECH：它们不在 Cloudflare 之后。
func TestNewRoutedTransportSkipsECHForInapplicableHosts(t *testing.T) {
	rt := NewRoutedTransport(defaultBaseTransport())
	rs, ok := rt.(*routedTransport)
	require.True(t, ok)

	for _, host := range []string{"i.pximg.net", "example.com"} {
		if route, routed := rs.routes[host]; routed {
			_, isECH := route.(*echTransport)
			assert.False(t, isECH, "主机 %s 不在 Cloudflare 之后，不应被施加 ECH", host)
		}
	}
}

// TestRoutedTransportRoutesECHHostsToECHTransport 断言清单内的主机确实被路由到
// 带 ECH 的传输，而不是常规传输。
func TestRoutedTransportRoutesECHHostsToECHTransport(t *testing.T) {
	base := defaultBaseTransport()
	rt := newRoutedTransport(base)

	for host := range echHostnames {
		route, ok := rt.routes[host]
		require.True(t, ok, "主机 %s 应有专门的传输", host)
		_, isECH := route.(*echTransport)
		assert.True(t, isECH, "主机 %s 应被路由到 ECH 传输，实际 %T", host, route)
	}
	// 不适用 ECH 的主机仍走不发送 SNI 的方式。
	_, ok := rt.routes["i.pximg.net"]
	assert.True(t, ok, "i.pximg.net 应保留不发送 SNI 的方式")
}

// TestHasRoutedWay 断言只有确有特殊方式的主机才需要回落重试。
func TestHasRoutedWay(t *testing.T) {
	for host, want := range map[string]bool{
		"www.pixiv.net":     true,
		"app-api.pixiv.net": true,
		"i.pximg.net":       true,
		"example.com":       false,
		"":                  false,
	} {
		assert.Equal(t, want, hasRoutedWay(host), "主机 %q", host)
	}
}

// TestAutoTransportFallsBackForECHHost 断言 ECH 不可用时自动选择仍能让请求成功：
// ECH 只在部分主机与网络环境下可用，失败后应回落常规连接。
func TestAutoTransportFallsBackForECHHost(t *testing.T) {
	var baseCalls int32
	base := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&baseCalls, 1)
		return okResponse(req), nil
	})
	rt := &AutoTransport{Base: base}
	// 让 ECH 方式必然失败，观察最终结果。
	rt.once.Do(func() {
		rt.base = base
		rt.routed = newRoutedTransportWithRoutes(base, map[string]http.RoundTripper{
			"www.pixiv.net": roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				return nil, errors.New("stub: ECH 不可用")
			}),
		})
	})

	req, err := http.NewRequest(http.MethodGet, "https://www.pixiv.net/", nil)
	require.NoError(t, err)
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err, "ECH 不可用时应回落到常规连接")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, int32(1), atomic.LoadInt32(&baseCalls))
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

// TestNewRoutedTransportWithPlainRoundTripper 断言基础传输不是 *http.Transport
// 时公开入口仍可用，请求照常发出。
func TestNewRoutedTransportWithPlainRoundTripper(t *testing.T) {
	var count int32
	rt := NewRoutedTransport(roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&count, 1)
		return okResponse(req), nil
	}))
	req, err := http.NewRequest(http.MethodGet, "https://i.pximg.net/x.png", nil)
	require.NoError(t, err)
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, int32(1), atomic.LoadInt32(&count))
}

// TestNewRoutedTransportNilBase 断言未提供基础传输时也能工作。
func TestNewRoutedTransportNilBase(t *testing.T) {
	assert.NotNil(t, NewRoutedTransport(nil))
}

// testTLSServer 启动一个本地 TLS 服务端，并返回一个把指定主机名拨到该
// 服务端的传输。请求因此使用真实主机名（而非 IP 字面量），TLS 的 SNI
// 行为才与真实场景一致——IP 字面量本身就不会产生 SNI 扩展。
func testTLSServer(t *testing.T, serverName *atomic.Value) (base *http.Transport, rawURL string) {
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
	// 两个用例只关心 SNI 是否发送，因此跳过证书校验：真实主机名与本地
	// 服务端的自签证书并不匹配，校验会先于本次断言的目标失败。
	base.TLSClientConfig.InsecureSkipVerify = true
	return base, "https://sni-probe.invalid/"
}

// TestTransportPrimitivesCompose 断言原语嵌套后请求按预期经过各层：
// 经两层无 SNI 原语发出的请求仍能完成握手，且服务端观测不到 SNI。
func TestTransportPrimitivesCompose(t *testing.T) {
	var serverName atomic.Value
	base, rawURL := testTLSServer(t, &serverName)

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
	base, rawURL := testTLSServer(t, &serverName)

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
	base, rawURL := testTLSServer(t, &serverName)

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
	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	rt = NewNoSNITransport(defaultBaseTransport())
	rt.TLSClientConfig.RootCAs = pool
	rt.TLSClientConfig.VerifyPeerCertificate = verifyPeerCertificate("", pool)
	client = &http.Client{Transport: rt}
	resp, err := client.Get(server.URL)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestAutoTransportAggregatesErrorsWhenAllWaysFail 断言自动选择在全部方式
// 都不可用时把每种方式的失败原因一起返回给调用者。
func TestAutoTransportAggregatesErrorsWhenAllWaysFail(t *testing.T) {
	rt := &AutoTransport{
		Base: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("stub: 不可用")
		}),
	}
	// 让受特殊处理的主机的首选方式也必然失败，迫使两种方式全部尝试。
	rt.once.Do(func() {
		rt.base = rt.Base
		rt.routed = newRoutedTransportWithRoutes(rt.Base, map[string]http.RoundTripper{
			"i.pximg.net": roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				return nil, errors.New("stub: 特殊方式不可用")
			}),
		})
	})
	req, err := http.NewRequest(http.MethodGet, "https://i.pximg.net/x.png", nil)
	require.NoError(t, err)
	_, err = rt.RoundTrip(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stub: 特殊方式不可用")
	assert.Contains(t, err.Error(), "stub: 不可用")
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
	rt := &AutoTransport{Base: base}
	// 让受特殊处理的主机的首选方式必然失败，观察最终结果。
	rt.once.Do(func() {
		rt.base = base
		rt.routed = newRoutedTransportWithRoutes(base, map[string]http.RoundTripper{
			"i.pximg.net": roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				return nil, errors.New("stub: 特殊方式不可用")
			}),
		})
	})

	req, err := http.NewRequest(http.MethodGet, "https://i.pximg.net/x.png", nil)
	require.NoError(t, err)
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err, "某个方式不可用时应继续尝试其余方式")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
