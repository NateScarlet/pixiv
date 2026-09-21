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
