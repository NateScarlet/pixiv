package connectivity

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProbeDoHDirectBranchIgnoresEnvProxy 断言 DoH 直连分支不受进程代理
// 环境变量影响：端点服务端收到请求即成功，代理服务端不应被触及。
func TestProbeDoHDirectBranchIgnoresEnvProxy(t *testing.T) {
	var endpointCalled, proxyCalled bool
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		endpointCalled = true
		w.Header().Set("Content-Type", "application/dns-json")
		fmt.Fprint(w, `{"Answer":[{"type":1,"data":"210.140.139.129"}]}`)
	}))
	defer endpoint.Close()

	proxy := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		proxyCalled = true
	}))
	defer proxy.Close()
	t.Setenv("HTTPS_PROXY", proxy.URL)

	p := liveProber{}
	ips, err := p.ProbeDoH(context.Background(), endpoint.URL, "i.pximg.net", false)
	require.NoError(t, err)
	assert.False(t, proxyCalled, "直连分支不应经代理")
	assert.True(t, endpointCalled, "应直接访问 DoH 端点")
	assert.NotEmpty(t, ips)
}

// TestProbeDoHProxyBranchForcesProxy 断言 DoH 经代理分支强制把请求发往
// 注入的代理地址：由伪装代理直接应答 DoH 结果，请求到达即证明走了代理。
func TestProbeDoHProxyBranchForcesProxy(t *testing.T) {
	var endpointCalled, proxyCalled bool
	endpoint := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		endpointCalled = true
	}))
	defer endpoint.Close()

	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyCalled = true
		assert.Equal(t, endpoint.URL, "http://"+r.Host, "代理应收到发往 DoH 端点的请求")
		fmt.Fprint(w, `{"Answer":[{"type":1,"data":"127.0.0.1"}]}`)
	}))
	defer proxy.Close()

	proxyURL, err := url.Parse(proxy.URL)
	require.NoError(t, err)
	p := liveProber{proxy: proxyURL}
	ips, err := p.ProbeDoH(context.Background(), endpoint.URL, "i.pximg.net", true)
	require.NoError(t, err)
	assert.True(t, proxyCalled, "请求应经过代理")
	assert.False(t, endpointCalled, "该测试中代理不转发，端点不应被直接访问")
	assert.NotEmpty(t, ips)
}

// TestProbeDoHWithoutProxyContract 断言经代理分支在未配置代理时快速失败：
// 静默按直连处理会产出误导性的「DoH 无需代理」结论。
func TestProbeDoHWithoutProxyContract(t *testing.T) {
	p := liveProber{}
	_, err := p.ProbeDoH(context.Background(), "https://1.1.1.1/dns-query", "i.pximg.net", true)
	require.Error(t, err)
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

// TestProbeHTTPSWithoutProxyContract 断言经代理分支在未配置代理时快速失败。
func TestProbeHTTPSWithoutProxyContract(t *testing.T) {
	p := liveProber{}
	assert.Error(t, p.ProbeHTTPS(context.Background(), "https://www.pixiv.net/", true))
}
