package dns

import (
	"context"
	"encoding/binary"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewResolverSystemEndpoint 断言 dns: 与 dns:// 表示系统解析。
//
// 解析 localhost 只可能来自系统解析（hosts 文件）：DoH 端点不会为它给出
// 记录，因此「成功解析出地址」足以钉住走的是系统解析这条分支。
func TestNewResolverSystemEndpoint(t *testing.T) {
	for _, endpoint := range []string{"dns:", "dns://"} {
		t.Run(endpoint, func(t *testing.T) {
			ips, err := NewResolver(endpoint).Resolve(context.Background(), "localhost")
			require.NoError(t, err)
			assert.NotEmpty(t, ips)
		})
	}
}

// TestNewResolverTraditionalDNS 断言 dns://<ip>:<port> 走传统 DNS：
// 查询被发往声明的服务器，并采用它给出的地址。
func TestNewResolverTraditionalDNS(t *testing.T) {
	var want = net.ParseIP("203.0.113.7")
	server := startUDPDNSServer(t, "127.0.0.1", want)

	ips, err := NewResolver("dns://"+server).Resolve(context.Background(), "i.pximg.net")
	require.NoError(t, err)
	require.Len(t, ips, 1)
	assert.Equal(t, want.String(), ips[0].String())
}

// TestNewResolverTraditionalDNSIPv6Literal 断言 IPv6 字面量形式的服务器地址
// 可以写：方括号属于 URL 语法，拨号地址需要还原成 [::1]:port。
func TestNewResolverTraditionalDNSIPv6Literal(t *testing.T) {
	var want = net.ParseIP("203.0.113.7")
	server := startUDPDNSServer(t, "::1", want)

	ips, err := NewResolver("dns://"+server).Resolve(context.Background(), "i.pximg.net")
	require.NoError(t, err)
	require.Len(t, ips, 1)
	assert.Equal(t, want.String(), ips[0].String())
}

// TestEndpointUsesHTTP 断言「查询是否经 HTTP 发出」由 scheme 决定：
// 调用者（如连通性探测）据此判断是否存在「经代理」这一出网路径。
//
// 明文 DNS 走 UDP，系统解析走平台 API，两者都没有可经代理的 HTTP 请求。
func TestEndpointUsesHTTP(t *testing.T) {
	for _, tt := range []struct {
		name     string
		endpoint string
		want     bool
	}{
		{"https", "https://1.1.1.1/dns-query", true},
		{"http", "http://127.0.0.1:8053/dns-query", true},
		{"明文 DNS", "dns://1.1.1.1:53", false},
		{"系统解析", "dns:", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, EndpointUsesHTTP(tt.endpoint))
		})
	}

	t.Run("写法非法", func(t *testing.T) {
		// 与 NewResolver 同一套校验：调用者不必先自行探一遍。
		assert.Panics(t, func() { EndpointUsesHTTP("tls://1.1.1.1") })
	})
}

// TestNewResolverRejectsOptionsForNonHTTPEndpoints 断言解析器选项只对
// 经 HTTP 的端点有意义：明文 DNS 与系统解析没有「发出查询的 HTTP client」，
// 传了就是调用者理解错了，快速失败而不是静默忽略。
func TestNewResolverRejectsOptionsForNonHTTPEndpoints(t *testing.T) {
	hc := &http.Client{}
	assert.Panics(t, func() { NewResolver("dns://1.1.1.1", WithHTTPClient(hc)) })
	assert.Panics(t, func() { NewResolver("dns:", WithHTTPClient(hc)) })
}

// TestNewResolverPlainHTTPDoH 断言 http:// 端点同样按 DoH 处理。
//
// 本地 DoH 服务（如 dnscrypt-proxy 的明文监听）常以 http 提供，这是本变量
// 既有的可用取值——DoH 解析器本身不校验 scheme。scheme 分派若只认 https，
// 就会把这类既有配置挡在构造期。
func TestNewResolverPlainHTTPDoH(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/dns-json")
		_, _ = w.Write([]byte(`{"Answer":[{"type":1,"data":"203.0.113.7"}]}`))
	}))
	t.Cleanup(srv.Close)

	ips, err := NewResolver(srv.URL+"#type=json").Resolve(context.Background(), "i.pximg.net")
	require.NoError(t, err)
	require.Len(t, ips, 1)
	assert.Equal(t, "203.0.113.7", ips[0].String())
}

// TestNewResolverPanicsOnInvalidEndpoint 断言端点写法非法时快速失败。
//
// 静默回落会得到一个与实际配置无关的解析结果：例如 dns:1.1.1.1（漏写 //）
// 在 url.Parse 中进入 Opaque 字段，若按「主机为空即系统解析」处理，
// 用户会以为指定了服务器而实际走了系统解析。
//
// 端口缺省（53）不作为独立用例：本机 53 端口上是否有 DNS 服务取决于运行
// 环境，没有可靠的外部行为可断言。
func TestNewResolverPanicsOnInvalidEndpoint(t *testing.T) {
	for _, tt := range []struct {
		name     string
		endpoint string
		want     string
	}{
		{"空端点", "", `pixiv: dns: 端点地址 "" 缺少 scheme: 可用写法为 http(s)://…（DoH）、dns://<ip>[:port]（传统 DNS）、dns:（系统解析）`},
		{"无 scheme", "1.1.1.1", `pixiv: dns: 端点地址 "1.1.1.1" 缺少 scheme: 可用写法为 http(s)://…（DoH）、dns://<ip>[:port]（传统 DNS）、dns:（系统解析）`},
		{"未知 scheme", "tls://1.1.1.1", `pixiv: dns: 端点地址 "tls://1.1.1.1" 的 scheme "tls" 无效: 可用写法为 http(s)://…（DoH）、dns://<ip>[:port]（传统 DNS）、dns:（系统解析）`},
		{"漏写 //", "dns:1.1.1.1", `pixiv: dns: 端点地址 "dns:1.1.1.1" 缺少 //: 指定服务器写成 dns://<ip>[:port]，用系统解析写成 dns:`},
		{"服务器非 IP 字面量", "dns://dns.example.com", `pixiv: dns: 端点地址 "dns://dns.example.com" 的服务器 "dns.example.com" 不是 IP 字面量: 写成 dns://<ip>[:port]`},
		{"DoH 专有的 fragment", "dns://1.1.1.1#type=json", `pixiv: dns: 端点地址 "dns://1.1.1.1#type=json": dns 方式不接受 fragment: #type 是 DoH 的查询方式声明，明文 DNS 没有对应概念`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.PanicsWithValue(t, tt.want, func() { NewResolver(tt.endpoint) })
		})
	}
}

// startUDPDNSServer 在指定回环地址上起一个最小 DNS 服务器，对任何查询都
// 以一条 A 记录应答，返回其监听地址（host:port 形式）。
//
// 服务器回显请求的事务 ID 并照抄问题段：解析器会校验响应与请求对应，
// 固定 ID 的应答会被当作错配丢弃。
func startUDPDNSServer(t *testing.T, host string, answer net.IP) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", net.JoinHostPort(host, "0"))
	if err != nil {
		t.Skipf("无法在 %s 上监听 UDP: %v", host, err)
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
func dnsAResponse(query []byte, answer net.IP) []byte {
	if len(query) < 12 {
		return nil
	}
	// 问题段之后紧接 QTYPE 与 QCLASS 各 2 字节。
	next, err := skipName(query, 12)
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

	// 应答记录：名字用压缩指针指向问题段的名字。
	var rr [16]byte
	rr[0], rr[1] = 0xc0, 0x0c
	binary.BigEndian.PutUint16(rr[2:4], dnsTypeA)
	binary.BigEndian.PutUint16(rr[4:6], dnsClassIN)
	binary.BigEndian.PutUint32(rr[6:10], 60)
	binary.BigEndian.PutUint16(rr[10:12], 4)
	copy(rr[12:16], answer.To4())

	resp := make([]byte, 0, end+len(rr))
	resp = append(resp, header[:]...)
	resp = append(resp, query[12:end]...)
	return append(resp, rr[:]...)
}
