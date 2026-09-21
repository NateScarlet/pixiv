package dns

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/NateScarlet/pixiv/internal/testenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDOHResolver(t *testing.T) {
	testenv.RequireLive(t)
	// 用默认端点验证解析机制本身即可；其他第三方端点的可用性属于部署环境，
	// 不是本库的代码行为（可用服务清单见 docs/env-vars.rst）。
	r := NewDOHResolver("https://1.1.1.1/dns-query")
	ip, err := r.Resolve(context.Background(), "www.pixiv.net")
	require.NoError(t, err)
	assert.NotEmpty(t, ip)
	assert.Regexp(t, `\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`, ip[0].String())
}

// TestDOHResolverUsesInjectedClient 断言注入的 HTTP client 被用于查询：
// 未注入时走 http.DefaultClient（遵循进程代理环境变量）；
// 注入后走调用者提供的 client，使探测类调用者可以做受控的代理分支实验。
func TestDOHResolverUsesInjectedClient(t *testing.T) {
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/dns-json")
		_, _ = w.Write([]byte(`{"Answer":[{"type":1,"data":"210.140.139.129"}]}`))
	}))
	t.Cleanup(srv.Close)

	used := &http.Client{}
	r := NewDOHResolverWithClient(srv.URL, used)
	ips, err := r.Resolve(context.Background(), "i.pximg.net")
	require.NoError(t, err)
	assert.True(t, called, "查询应经注入的 client 发出")
	got := make([]string, 0, len(ips))
	for _, ip := range ips {
		got = append(got, ip.String())
	}
	assert.Equal(t, []string{"210.140.139.129"}, got)
}

// TestDOHResolverResolvesOwnProxyViaSystem 断言解析「自身访问链路上的代理地址」时
// 使用系统解析，避免递归：DoH 查询本身要经该代理出网，若再经 DoH 解析代理名，
// 就是「代理可达以 DoH 可用为前提、DoH 可用又以代理可达为前提」的自举死锁。
//
// 场景还原：HTTPS_PROXY 指向主机名形式的本地代理，无 SNI 传输的解析接缝
// 会把代理地址交给解析器。本用例直接从解析器语义层钉住：解析该代理名时
// 不发出 DoH 查询，而是走系统解析——端点故意指向不可达地址也不会被触及。
func TestDOHResolverResolvesOwnProxyViaSystem(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://localhost:7890")
	r := NewDOHResolverWithClient("http://127.0.0.1:1/dns-query", &http.Client{})
	ip, err := r.Resolve(context.Background(), "localhost")
	require.NoError(t, err)
	assert.NotEmpty(t, ip, "应回落系统解析得到结果")
}
