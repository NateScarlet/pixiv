package client

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"os"
	"testing"

	"github.com/NateScarlet/pixiv/internal/testenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ECH 的用途是**直连**时绕开按 SNI 的封锁，因此本文件的用例必须让数据连接
// 直连发出；代理只用来查询 DoH（本机网络下境外 DoH 端点不可达，见
// docs/direct-connection.rst），绝不用来承载被 ECH 保护的请求——
// 经代理时封锁本就被绕过，那样测不出 ECH 是否生效。
//
// 这些用例断言真实 pixiv 与真实网络当下的行为，失败可能源于 pixiv 改版或
// 封锁策略变化，故加真实网络门禁：置 PIXIV_LIVE=1 才运行。
//
// 需要能查询境外 DoH：置 PIXIV_TEST_PROXY 指向一个可用代理，
// 该代理仅用于 DoH 请求（DoH 使用 http.DefaultClient，遵循 HTTPS_PROXY）。

// directBaseTransport 返回数据连接直连、主机名经 DoH 解析的传输。
//
// 它不使用代理：代理会绕过封锁，从而让 ECH 的验证失去意义。
func directBaseTransport(t *testing.T) *http.Transport {
	t.Helper()

	proxyRaw := os.Getenv("PIXIV_TEST_PROXY")
	require.NotEmpty(t, proxyRaw,
		"需要代理查询境外 DoH；它只用于 DoH，不承载被 ECH 保护的请求")

	// DoH 走 http.DefaultClient，它遵循代理环境变量；数据连接不经代理。
	t.Setenv("HTTPS_PROXY", proxyRaw)
	t.Setenv("HTTP_PROXY", proxyRaw)

	resolver := defaultDNSResolver()
	base := defaultBaseTransport()
	base.Proxy = nil
	base.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		_, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		// 用 DoH 的结果替换主机名，避免系统解析返回被污染的地址。
		ips, err := resolver.Resolve(ctx, "www.pixiv.net")
		if err != nil {
			return nil, err
		}
		require.NotEmpty(t, ips, "DoH 未返回可用地址")
		var d net.Dialer
		return d.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
	}
	return base
}

// TestECHTransportDirectBypassesSNIBlocking 断言直连时施加 ECH 让请求成功，
// 而同样直连、不施加 ECH 时该请求失败。
//
// 这是 ECH 的用途：封锁按 SNI 关键词匹配，ECH 把真实域名加密在内层，
// 中间设备只能看到外层名。
func TestECHTransportDirectBypassesSNIBlocking(t *testing.T) {
	testenv.RequireLive(t)

	// 对照：与 ECH 用例完全相同的直连方式，只是不施加 ECH。
	plain := New(WithTransport(directBaseTransport(t)))
	plainResp, plainErr := plain.Get("https://www.pixiv.net/")
	if plainResp != nil {
		plainResp.Body.Close()
	}
	if plainErr == nil && plainResp.StatusCode == http.StatusOK {
		t.Skip("当前网络未对该主机施加 SNI 封锁，本用例无法体现 ECH 的作用")
	}
	t.Logf("对照（不施加 ECH）: 错误=%v", plainErr)

	// 施加 ECH：应当成功，且 ECH 被真正接受。
	base := directBaseTransport(t)
	var accepted bool
	// 在 base 上观测：原语不劫持调用者的 VerifyConnection。
	base.TLSClientConfig.VerifyConnection = func(cs tls.ConnectionState) error {
		accepted = cs.ECHAccepted
		return nil
	}
	c := New(WithTransport(NewECHTransport(base, WithECHPublicName(ECHDefaultPublicName))))

	resp, err := c.Get("https://www.pixiv.net/")
	require.NoError(t, err, "直连施加 ECH 后应当成功")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.True(t, accepted, "ECH 应被真正接受，而不是静默回退")
}
