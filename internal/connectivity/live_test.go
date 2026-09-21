package connectivity

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/NateScarlet/pixiv/pkg/client/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestQueryDoHParsesARecords 断言 DoH 查询解析 JSON 应答中的 A 记录，
// 并忽略其它类型的记录（与库内解析器的判读一致）。
func TestQueryDoHParsesARecords(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/dns-json")
		// type 28 是 AAAA 记录，不应被当作 A 记录返回。
		fmt.Fprint(w, `{"Answer":[`+
			`{"name":"i.pximg.net","type":1,"data":"210.140.139.129"},`+
			`{"name":"i.pximg.net","type":28,"data":"2406:da14::1"}]}`)
	}))
	defer srv.Close()

	ips, err := queryDoH(context.Background(), srv.URL, "i.pximg.net", nil)
	require.NoError(t, err)
	assert.Equal(t, []net.IP{net.ParseIP("210.140.139.129")}, ips,
		"应只返回 A 记录的地址")
}

// TestQueryDoHRejectsNonOKStatus 断言非 200 应答被视为查询失败。
func TestQueryDoHRejectsNonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	_, err := queryDoH(context.Background(), srv.URL, "i.pximg.net", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "403")
}

// TestQueryDoHViaProxyReachesProxy 断言经代理查询时请求确实发往代理地址：
// 由该「代理」服务端直接应答 DoH 结果，请求到达即说明代理路径被强制使用。
func TestQueryDoHViaProxyReachesProxy(t *testing.T) {
	var endpointCalled, proxyCalled bool
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		endpointCalled = true
		fmt.Fprint(w, `{"Answer":[]}`)
	}))
	defer endpoint.Close()

	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyCalled = true
		// 伪装代理：不转发，直接以 DoH 应答。请求到达即证明走了代理。
		assert.Equal(t, endpoint.URL, "http://"+r.Host, "代理应收到发往 DoH 端点的请求")
		fmt.Fprint(w, `{"Answer":[{"type":1,"data":"127.0.0.1"}]}`)
	}))
	defer proxy.Close()

	proxyURL, err := url.Parse(proxy.URL)
	require.NoError(t, err)
	ips, err := queryDoH(context.Background(), endpoint.URL, "i.pximg.net", proxyURL)
	require.NoError(t, err)
	assert.True(t, proxyCalled, "请求应经过代理")
	assert.False(t, endpointCalled, "该测试中代理不转发，端点不应被直接访问")
	assert.Equal(t, []net.IP{net.ParseIP("127.0.0.1")}, ips)
}

// TestProbeHTTPSAnyStatusIsSuccess 断言探测以「收到 HTTP 应答」为成功标准：
// 连接层可用即成功，业务状态码（如图片主机对无 Referer 请求的 403）
// 不构成探测失败。
func TestProbeHTTPSAnyStatusIsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	p := liveProber{}
	assert.NoError(t, p.ProbeHTTPS(context.Background(), srv.URL, false))
}

// TestProbeHTTPSReportsTransportFailure 断言连接层失败被作为探测失败返回。
func TestProbeHTTPSReportsTransportFailure(t *testing.T) {
	// 指向已关闭的端口：拨号必然失败。
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())

	p := liveProber{}
	err = p.ProbeHTTPS(context.Background(), "http://"+addr+"/", false)
	require.Error(t, err)
}

// TestProbeHTTPSWithProxyReachesProxy 断言经代理探测时请求发往代理地址。
//
// 用 http 目标验证：标准库对 http 目标以代理表单（绝对 URL 请求行）把请求
// 直接发给代理，伪装代理可以直接应答，请求到达即证明代理路径被强制使用。
// https 目标经代理时标准库发 CONNECT 建隧道，隧道行为由标准库保证。
func TestProbeHTTPSWithProxyReachesProxy(t *testing.T) {
	var proxyCalled bool
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyCalled = true
		assert.Equal(t, "http://www.pixiv.net/", r.RequestURI,
			"代理应收到代理表单的请求")
		w.WriteHeader(http.StatusOK)
	}))
	defer proxy.Close()

	proxyURL, err := url.Parse(proxy.URL)
	require.NoError(t, err)

	p := liveProber{proxy: proxyURL}
	assert.NoError(t, p.ProbeHTTPS(context.Background(), "http://www.pixiv.net/", true))
	assert.True(t, proxyCalled, "请求应经过代理")
}

// TestDialViaResolverResolvesHostnames 断言拨号包装器经解析器解析主机名，
// 并拨向解析得到的地址。
func TestDialViaResolverResolvesHostnames(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	require.NoError(t, err)

	var resolved []string
	resolver := resolverFunc(func(_ context.Context, host string) ([]net.IP, error) {
		resolved = append(resolved, host)
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	})

	conn, err := dialViaResolver(resolver)(context.Background(), "tcp", net.JoinHostPort("i.pximg.net", port))
	require.NoError(t, err)
	defer conn.Close()
	assert.Equal(t, []string{"i.pximg.net"}, resolved, "主机名应经解析器解析")
}

// TestDialViaResolverSkipsIPLiterals 断言 IP 字面量不经过解析器直接拨号：
// 与库内解析接缝的语义一致，经代理拨号代理地址时依赖这一点。
func TestDialViaResolverSkipsIPLiterals(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	resolver := resolverFunc(func(context.Context, string) ([]net.IP, error) {
		t.Error("IP 字面量不应触发解析")
		return nil, nil
	})

	conn, err := dialViaResolver(resolver)(context.Background(), "tcp", srv.Listener.Addr().String())
	require.NoError(t, err)
	require.NoError(t, conn.Close())
}

// TestDialViaResolverReportsResolveFailure 断言解析失败给出指明排查方向的错误。
func TestDialViaResolverReportsResolveFailure(t *testing.T) {
	resolver := resolverFunc(func(context.Context, string) ([]net.IP, error) {
		return nil, fmt.Errorf("DoH 不可达")
	})
	_, err := dialViaResolver(resolver)(context.Background(), "tcp", "i.pximg.net:443")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DoH 不可达")
	assert.Contains(t, err.Error(), "i.pximg.net")
}

// TestLiveProberProxyContract 断言经代理探测在未配置代理时快速失败：
// 这是装配错误，静默按直连处理会产出误导性结论。
func TestLiveProberProxyContract(t *testing.T) {
	p := liveProber{}
	_, err := p.ProbeDoH(context.Background(), "https://1.1.1.1/dns-query", "i.pximg.net", true)
	assert.Error(t, err)
	err = p.ProbeHTTPS(context.Background(), "https://www.pixiv.net/", true)
	assert.Error(t, err)
}

// resolverFunc 把函数适配为 dns.Resolver，与库内测试的惯用法一致。
type resolverFunc func(ctx context.Context, host string) ([]net.IP, error)

func (f resolverFunc) Resolve(ctx context.Context, host string) ([]net.IP, error) {
	return f(ctx, host)
}

// 编译期确认 resolverFunc 满足接口。
var _ dns.Resolver = resolverFunc(nil)
