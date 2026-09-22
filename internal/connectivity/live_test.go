package connectivity

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProbeResolverUsesEndpointDeclaredWireFormat 断言探测按端点 URL 声明的
// 编码方式发出查询。
//
// 探测与运行时取自同一端点字符串，因此运行时能用的写法探测也能用：
// 端点只支持 RFC 8484 二进制接口时（如 dnscrypt-proxy 本地 DoH），
// 用 JSON 方式探测会得到一个「端点拒绝查询」的结论，而运行时其实可用。
func TestProbeResolverUsesEndpointDeclaredWireFormat(t *testing.T) {
	newEndpoint := func() (*httptest.Server, *string, *string) {
		var gotDNS, gotName string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			gotDNS = req.URL.Query().Get("dns")
			gotName = req.URL.Query().Get("name")
			if gotDNS == "" {
				// 只实现 RFC 8484：没有 dns 参数即拒绝。
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/dns-message")
			_, _ = w.Write([]byte{
				0x00, 0x00, 0x81, 0x80, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00,
				1, 'i', 5, 'p', 'x', 'i', 'm', 'g', 3, 'n', 'e', 't', 0,
				0x00, 0x01, 0x00, 0x01,
				0xc0, 0x0c, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x3c, 0x00, 0x04,
				210, 140, 139, 129,
			})
		}))
		return srv, &gotDNS, &gotName
	}

	t.Run("默认二进制", func(t *testing.T) {
		srv, gotDNS, gotName := newEndpoint()
		defer srv.Close()

		p := liveProber{}
		ips, err := p.ProbeResolver(context.Background(), srv.URL, "i.pximg.net", false)
		require.NoError(t, err)
		assert.NotEmpty(t, *gotDNS, "默认应按二进制报文方式发出查询")
		assert.Empty(t, *gotName, "二进制方式不应发 name 参数")
		require.Len(t, ips, 1)
		assert.Equal(t, "210.140.139.129", ips[0].String())
	})

	t.Run("fragment 不发给服务端", func(t *testing.T) {
		srv, gotDNS, _ := newEndpoint()
		defer srv.Close()

		p := liveProber{}
		_, err := p.ProbeResolver(context.Background(), srv.URL+"#type=message", "i.pximg.net", false)
		require.NoError(t, err)
		assert.NotEmpty(t, *gotDNS)
	})
}

// TestProbeResolverDirectBranchIgnoresEnvProxy 断言解析直连分支不受进程代理
// 环境变量影响：端点服务端收到请求即成功，代理服务端不应被触及。
func TestProbeResolverDirectBranchIgnoresEnvProxy(t *testing.T) {
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
	// 该伪端点以 JSON 对答，因此按 JSON 方式查询；本用例关心的是代理路径。
	ips, err := p.ProbeResolver(context.Background(), endpoint.URL+"#type=json", "i.pximg.net", false)
	require.NoError(t, err)
	assert.False(t, proxyCalled, "直连分支不应经代理")
	assert.True(t, endpointCalled, "应直接访问 DoH 端点")
	assert.NotEmpty(t, ips)
}

// TestProbeResolverProxyBranchForcesProxy 断言解析经代理分支强制把请求发往
// 注入的代理地址：由伪装代理直接应答 DoH 结果，请求到达即证明走了代理。
func TestProbeResolverProxyBranchForcesProxy(t *testing.T) {
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
	// 该伪代理以 JSON 对答，因此按 JSON 方式查询；本用例关心的是走了代理。
	ips, err := p.ProbeResolver(context.Background(), endpoint.URL+"#type=json", "i.pximg.net", true)
	require.NoError(t, err)
	assert.True(t, proxyCalled, "请求应经过代理")
	assert.False(t, endpointCalled, "该测试中代理不转发，端点不应被直接访问")
	assert.NotEmpty(t, ips)
}

// TestProbeResolverWithoutProxyContract 断言经代理分支在未配置代理时快速失败：
// 静默按直连处理会产出误导性的「无需代理」结论。
func TestProbeResolverWithoutProxyContract(t *testing.T) {
	p := liveProber{}
	_, err := p.ProbeResolver(context.Background(), "https://1.1.1.1/dns-query", "i.pximg.net", true)
	require.Error(t, err)
}

// TestProbeResolverTraditionalDNS 断言 dns:// 端点按明文 DNS 探测：
// 查询被发往声明的服务器，并采用它给出的地址。探测与运行时同一构造，
// 因此这里能解析出地址就说明运行时同样可用。
func TestProbeResolverTraditionalDNS(t *testing.T) {
	var want = net.ParseIP("203.0.113.7")
	server := startUDPDNSServer(t, want)

	p := liveProber{}
	ips, err := p.ProbeResolver(context.Background(), "dns://"+server, "i.pximg.net", false)
	require.NoError(t, err)
	require.Len(t, ips, 1)
	assert.Equal(t, want.String(), ips[0].String())
}

// TestProbeResolverNonHTTPRejectsProxyPath 断言明文 DNS 与系统解析没有
// 「经代理」这条路径：它们走 UDP 或平台 API，不受进程代理环境影响。
// 静默按直连处理会产出误导性的「无需代理」结论。
func TestProbeResolverNonHTTPRejectsProxyPath(t *testing.T) {
	proxyURL, err := url.Parse("http://127.0.0.1:7890")
	require.NoError(t, err)
	p := liveProber{proxy: proxyURL}

	for _, endpoint := range []string{"dns://1.1.1.1", "dns:"} {
		t.Run(endpoint, func(t *testing.T) {
			_, err := p.ProbeResolver(context.Background(), endpoint, "i.pximg.net", true)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "不经 HTTP")
		})
	}
}

// startUDPDNSServer 在回环地址上起一个最小 DNS 服务器，对任何查询都以
// 一条 A 记录应答，返回其监听地址（host:port 形式）。
func startUDPDNSServer(t *testing.T, answer net.IP) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("无法监听 UDP: %v", err)
	}
	t.Cleanup(func() { _ = pc.Close() })

	go func() {
		buf := make([]byte, 512)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			if resp := dnsAResponse(buf[:n], answer); resp != nil {
				if _, err := pc.WriteTo(resp, from); err != nil {
					return
				}
			}
		}
	}()
	return pc.LocalAddr().String()
}

// dnsAResponse 构造一个「问题段照抄、应答段含一条 A 记录」的响应。
// 事务 ID 回显是必需的：标准库的 DNS 客户端会校验响应与请求对应。
func dnsAResponse(query []byte, answer net.IP) []byte {
	if len(query) < 12 {
		return nil
	}
	// 问题段之后紧接 QTYPE 与 QCLASS 各 2 字节。
	next, err := skipDNSName(query, 12)
	if err != nil {
		return nil
	}
	end := next + 4
	if end > len(query) {
		return nil
	}

	var header [12]byte
	copy(header[0:2], query[0:2])                   // 回显事务 ID
	binary.BigEndian.PutUint16(header[2:4], 0x8180) // QR=1, RD=1, RA=1, RCODE=0
	binary.BigEndian.PutUint16(header[4:6], 1)      // QDCOUNT
	binary.BigEndian.PutUint16(header[6:8], 1)      // ANCOUNT

	var rr [16]byte
	rr[0], rr[1] = 0xc0, 0x0c                // 名字用压缩指针指向问题段
	binary.BigEndian.PutUint16(rr[2:4], 1)   // TYPE=A
	binary.BigEndian.PutUint16(rr[4:6], 1)   // CLASS=IN
	binary.BigEndian.PutUint32(rr[6:10], 60) // TTL
	binary.BigEndian.PutUint16(rr[10:12], 4) // RDLENGTH
	copy(rr[12:16], answer.To4())

	resp := make([]byte, 0, end+len(rr))
	resp = append(resp, header[:]...)
	resp = append(resp, query[12:end]...)
	return append(resp, rr[:]...)
}

// skipDNSName 跳过报文中的域名，返回其后的偏移量。
func skipDNSName(msg []byte, offset int) (int, error) {
	for {
		if offset >= len(msg) {
			return 0, errors.New("域名越界")
		}
		length := int(msg[offset])
		switch {
		case length == 0:
			return offset + 1, nil
		case length&0xc0 == 0xc0:
			return offset + 2, nil
		default:
			offset += 1 + length
		}
	}
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
