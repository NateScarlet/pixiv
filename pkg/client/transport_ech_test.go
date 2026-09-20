package client

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 本文件的测试只断言公开入口在真实握手下的外部可观测行为：
// ECH 是否被服务端接受、被拒时返回什么错误、经代理时是否仍然生效。
// 不断言内部字段、重试次数或配置缓存方式。

// 真实域名与 public_name 的选择受证书约束。
//
// httptest 内置证书的 SAN 为 127.0.0.1、::1、example.com、*.example.com。
// 两条校验路径用到不同的名字：ECH 被接受时用内层真实域名校验证书，
// 被拒时用外层 public_name 校验证书。故让两者都落在证书覆盖范围内。
const (
	echTestRealHost   = "example.com"
	echTestPublicName = "ech.public.example.com"
)

// #region 测试服务端

// echTestServer 是启用了 ECH 的本地 TLS 服务端。
type echTestServer struct {
	srv      *httptest.Server
	addr     string
	pool     *x509.CertPool
	config   []byte
	sniMu    sync.Mutex
	outerSNI []string // 解密前的外层 SNI
	innerSNI []string // 解密后的内层 SNI
}

// newECHTestServer 启动启用了 ECH 的本地 TLS 服务端。
//
// sendAsRetry 决定该密钥是否随拒绝一并下发，即服务端是否提供 retry_configs。
func newECHTestServer(t *testing.T, sendAsRetry bool) *echTestServer {
	t.Helper()
	echKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	require.NoError(t, err)
	config, err := marshalECHConfig(185, echKey.PublicKey().Bytes(), echTestPublicName)
	require.NoError(t, err)

	es := &echTestServer{config: config}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS != nil {
			es.sniMu.Lock()
			es.innerSNI = append(es.innerSNI, r.TLS.ServerName)
			es.sniMu.Unlock()
		}
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewUnstartedServer(handler)
	// 只协商 http/1.1，使经代理的用例不需要在代理侧处理 HTTP/2 帧。
	srv.EnableHTTP2 = false
	srv.TLS = &tls.Config{
		MinVersion: tls.VersionTLS13,
		NextProtos: []string{"http/1.1"},
		// GetEncryptedClientHelloKeys 在解密之前调用，此处看到的 ServerName
		// 才是外层名；GetConfigForClient 与 handler 只能看到内层名。
		GetEncryptedClientHelloKeys: func(chi *tls.ClientHelloInfo) ([]tls.EncryptedClientHelloKey, error) {
			es.sniMu.Lock()
			es.outerSNI = append(es.outerSNI, chi.ServerName)
			es.sniMu.Unlock()
			return []tls.EncryptedClientHelloKey{
				{Config: es.config, PrivateKey: echKey.Bytes(), SendAsRetry: sendAsRetry},
			}, nil
		},
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)

	es.srv = srv
	es.addr = srv.Listener.Addr().String()
	es.pool = x509.NewCertPool()
	es.pool.AddCert(srv.Certificate())
	return es
}

// newServerTLS 返回一份信任该测试服务端证书的 TLS 配置。
//
// 用途是把测试服务端的自签证书放入根证书集，使证书校验按真实域名通过；
// ECH 相关的字段由被测原语自行设置。
func (es *echTestServer) newServerTLS() *tls.Config {
	return &tls.Config{
		RootCAs:    es.pool,
		MinVersion: tls.VersionTLS13,
		NextProtos: []string{"http/1.1"},
	}
}

// seenOuterSNI 返回服务端解密前观测到的外层 SNI。
func (es *echTestServer) seenOuterSNI() []string {
	es.sniMu.Lock()
	defer es.sniMu.Unlock()
	return append([]string(nil), es.outerSNI...)
}

// seenInnerSNI 返回服务端解密后观测到的内层 SNI。
func (es *echTestServer) seenInnerSNI() []string {
	es.sniMu.Lock()
	defer es.sniMu.Unlock()
	return append([]string(nil), es.innerSNI...)
}

// transport 返回指向该测试服务端、信任其证书的传输。
func (es *echTestServer) transport() *http.Transport {
	return dialingTo(es.addr, es.newServerTLS())
}

// dialingTo 返回把底层 TCP 连接指向 addr 的传输。
// 请求因此使用真实主机名（而非 IP 字面量），TLS 的 SNI 与证书校验行为才与真实场景一致。
func dialingTo(addr string, tlsCfg *tls.Config) *http.Transport {
	return &http.Transport{
		TLSClientConfig: tlsCfg,
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, addr)
		},
	}
}

// #endregion

// TestECHTransportAppliesECH 断言原语确实施加了 ECH：
// 真实主机名被加密在内层，服务端在解密前只能看到外层名，且 ECH 被真正接受。
func TestECHTransportAppliesECH(t *testing.T) {
	es := newECHTestServer(t, true)

	rt := NewECHTransport(es.transport(), WithECHConfigList(echTestServerConfigList(t, es)))
	client := &http.Client{Transport: rt}

	resp, err := client.Get("https://" + echTestRealHost + "/")
	require.NoError(t, err)
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	outer := es.seenOuterSNI()
	require.NotEmpty(t, outer)
	assert.Equal(t, echTestPublicName, outer[0], "解密前网络侧应只看到外层名")

	inner := es.seenInnerSNI()
	require.NotEmpty(t, inner)
	assert.Equal(t, echTestRealHost, inner[0], "真实主机名应被加密在内层")
}

// TestECHTransportAcceptsProvidedConfigList 断言调用者提供的配置被采用并生效。
func TestECHTransportAcceptsProvidedConfigList(t *testing.T) {
	es := newECHTestServer(t, true)
	base := dialingTo(es.addr, es.newServerTLS())
	var seen echObserved
	observing(base, &seen)
	rt := NewECHTransport(base, WithECHConfigList(echTestServerConfigList(t, es)))

	resp, err := (&http.Client{Transport: rt}).Get("https://" + echTestRealHost + "/")
	require.NoError(t, err)
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.True(t, seen.accepted, "提供了正确配置时应真正建立 ECH 连接")
}

// TestECHTransportBootstrapsWithoutExtraLookup 断言未提供配置时原语自行取得配置，
// 且取配置这一步不引入额外的主机解析。
//
// 自举经 TLS 握手完成：发送一份服务端无法解密的配置，用 HelloRetryRequest 中
// 下发的 retry_configs 取回真正配置。这里把拨号指向测试服务端（因此不需要系统
// 解析也能连上），并注入一个记录调用的解析器：解析只应发生在目标主机上，
// 取配置本身不解析任何额外的名字。
func TestECHTransportBootstrapsWithoutExtraLookup(t *testing.T) {
	es := newECHTestServer(t, true)

	var mu sync.Mutex
	var resolved []string
	// 拨号指向测试服务端：连接建立不依赖解析，因此可用「解析被调用了哪些名字」
	// 判断取配置这一步是否引入了 DNS 依赖。
	base := es.transport()
	rt := NewECHTransport(base, WithECHPublicName(echTestPublicName))

	c := New(
		WithTransport(rt),
		WithDNSResolver(resolverFunc(func(ctx context.Context, host string) ([]net.IP, error) {
			mu.Lock()
			resolved = append(resolved, host)
			mu.Unlock()
			// 返回一个不可达地址，使解析被调用这件事本身不会让用例意外通过。
			return []net.IP{net.ParseIP("127.0.0.1")}, nil
		})),
	)
	resp, err := c.Get("https://" + echTestRealHost + "/")
	require.NoError(t, err, "未提供配置时应能自行取得")
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotEmpty(t, es.seenOuterSNI())
	assert.Equal(t, echTestPublicName, es.seenOuterSNI()[0])

	mu.Lock()
	defer mu.Unlock()
	for _, host := range resolved {
		assert.Equal(t, echTestRealHost, host, "取配置不应引入额外的解析目标")
	}
}

// TestECHTransportHealsAfterConfigRotation 断言配置轮换后原语能在运行时自愈：
// 使用一份服务端已不认识的配置时，服务端下发新配置，原语用它重试并成功，
// 不需要重启或外部文件。
func TestECHTransportHealsAfterConfigRotation(t *testing.T) {
	es := newECHTestServer(t, true)

	// 一份格式合法但服务端不持有其私钥的配置，等价于「轮换后过期的旧配置」。
	stale := echTestForeignConfigList(t)

	base := dialingTo(es.addr, es.newServerTLS())
	var seen echObserved
	observing(base, &seen)
	rt := NewECHTransport(base, WithECHConfigList(stale))
	resp, err := (&http.Client{Transport: rt}).Get("https://" + echTestRealHost + "/")
	require.NoError(t, err, "应下发新配置并重试成功")
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.True(t, seen.accepted, "自愈后应真正建立 ECH 连接")
}

// TestECHTransportBootstrapHappensOnce 断言配置只取一次：首次请求后重复使用同一
// 传输发起多次请求，服务端看到的握手数不再增长（连接复用），说明没有反复取配置。
//
// 这里不断言首次请求的握手次数：它包含自举握手、用 retry_configs 的重试握手
// 与真实连接的握手，次数由标准库的连接建立方式决定，不构成对外承诺。
func TestECHTransportBootstrapHappensOnce(t *testing.T) {
	es := newECHTestServer(t, true)
	rt := NewECHTransport(es.transport(), WithECHPublicName(echTestPublicName))
	c := &http.Client{Transport: rt}

	get := func() {
		t.Helper()
		resp, err := c.Get("https://" + echTestRealHost + "/")
		require.NoError(t, err)
		defer resp.Body.Close()
		io.Copy(io.Discard, resp.Body)
	}

	get()
	afterFirst := len(es.seenOuterSNI())
	for i := 0; i < 4; i++ {
		get()
	}
	assert.Equal(t, afterFirst, len(es.seenOuterSNI()),
		"已取得配置后不应再发起自举握手")
}

// TestECHTransportConcurrentBootstrapSharesOneFetch 断言并发的首批请求共用同一次
// 配置取得：否则每个请求都会各自发起一次自举握手。
//
// 判定依据是拨号次数：并发 N 个请求时，共用一次取配置的拨号数为 N（各自建立
// 真实连接）加 1（一次自举）；若各取一次则接近 2N。
func TestECHTransportConcurrentBootstrapSharesOneFetch(t *testing.T) {
	es := newECHTestServer(t, true)
	base := es.transport()
	var mu sync.Mutex
	var dials int
	inner := base.DialContext
	base.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		mu.Lock()
		dials++
		mu.Unlock()
		return inner(ctx, network, addr)
	}
	rt := NewECHTransport(base, WithECHPublicName(echTestPublicName))
	c := &http.Client{Transport: rt}

	const requests = 8
	var wg sync.WaitGroup
	errs := make(chan error, requests)
	start := make(chan struct{})
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // 尽量让请求同时开始，制造并发取配置的情形。
			resp, err := c.Get("https://" + echTestRealHost + "/")
			if err != nil {
				errs <- err
				return
			}
			defer resp.Body.Close()
			io.Copy(io.Discard, resp.Body)
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("并发请求失败: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	t.Logf("并发 %d 个请求，拨号次数=%d", requests, dials)
	// 上界取 N+2：N 次真实连接加一次自举，留出少量波动余量。
	// 若各请求各自取配置，这里会接近 2N。
	assert.LessOrEqual(t, dials, requests+2,
		"并发取配置应共用一次自举，否则拨号次数会接近请求数的两倍")
}

// TestECHTransportErrorsWhenRejectedWithoutRetry 断言服务端拒绝且未下发配置时
// 返回错误，而不是静默退回明文握手。
func TestECHTransportErrorsWhenRejectedWithoutRetry(t *testing.T) {
	// 服务端启用了 ECH 但拒绝下发重试配置：它拒绝 ECH，且不提供 retry_configs。
	es := newECHTestServer(t, false)
	rt := NewECHTransport(
		es.transport(),
		WithECHConfigList(echTestForeignConfigList(t)),
	)
	_, err := (&http.Client{Transport: rt}).Get("https://" + echTestRealHost + "/")
	require.Error(t, err, "被拒且无 retry_configs 时应报错，不得静默退回明文握手")
	assert.Contains(t, err.Error(), "ECH")
}

// TestECHTransportErrorsWhenNotBehindCloudflare 断言对不适用 ECH 的主机给出明确错误，
// 而不是让它以证书不匹配的形式失败。
//
// 该场景用一台不参与 ECH 的普通 TLS 服务端模拟「不在 Cloudflare 之后的主机」：
// 它的证书不覆盖 ECH 的外层名，因此 ECH 不适用。
func TestECHTransportErrorsWhenNotBehindCloudflare(t *testing.T) {
	plain := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	plain.EnableHTTP2 = false
	plain.TLS = &tls.Config{MinVersion: tls.VersionTLS13, NextProtos: []string{"http/1.1"}}
	plain.StartTLS()
	t.Cleanup(plain.Close)

	pool := x509.NewCertPool()
	pool.AddCert(plain.Certificate())

	rt := NewECHTransport(
		dialingTo(plain.Listener.Addr().String(), &tls.Config{
			RootCAs:    pool,
			MinVersion: tls.VersionTLS13,
			NextProtos: []string{"http/1.1"},
		}),
		WithECHConfigList(echTestForeignConfigList(t)),
	)
	_, err := (&http.Client{Transport: rt}).Get("https://" + echTestRealHost + "/")
	require.Error(t, err, "对该主机 ECH 不适用，应返回错误而不是尝试明文连接")
	// 错误应指明 ECH 相关原因，而不是让调用者面对一个语焉不详的握手失败。
	assert.Contains(t, err.Error(), "ECH")
}

// TestECHTransportAppliesECHWithoutDialTLS 断言原语在不使用 DialTLSContext 的前提下
// 确实施加了 ECH：DialTLSContext 只对 non-proxied 请求生效，经代理时会被静默忽略，
// 因此能力必须由 TLSClientConfig 承载。
//
// 断言的是外部可观测结果：ECH 被服务端接受；若能力靠 DialTLSContext 施加，
// 这一点在没有代理时也可能「看起来能用」，故配合经代理的用例一起约束。
func TestECHTransportAppliesECHWithoutDialTLS(t *testing.T) {
	es := newECHTestServer(t, true)
	base := dialingTo(es.addr, es.newServerTLS())
	var seen echObserved
	observing(base, &seen)
	rt := NewECHTransport(base, WithECHConfigList(echTestServerConfigList(t, es)))

	resp, err := (&http.Client{Transport: rt}).Get("https://" + echTestRealHost + "/")
	require.NoError(t, err)
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	assert.True(t, seen.accepted, "ECH 应由 TLSClientConfig 施加并真正生效")
	// 原语不得改动调用者传输的拨号能力：DialTLSContext 在代理下会被静默忽略。
	assert.Nil(t, base.DialTLSContext, "不应通过 DialTLSContext 施加能力")
	assert.Nil(t, base.DialTLS)
}

// TestECHTransportDoesNotMutateBase 断言原语不改变调用者的传输。
func TestECHTransportDoesNotMutateBase(t *testing.T) {
	base := defaultBaseTransport()
	baseTLS := base.TLSClientConfig.Clone()

	rt := NewECHTransport(base, WithECHConfigList(echTestForeignConfigList(t)))
	require.NotNil(t, rt)

	assert.Nil(t, base.TLSClientConfig.EncryptedClientHelloConfigList, "不应改变调用者基础传输的配置")
	assert.Equal(t, baseTLS.MinVersion, base.TLSClientConfig.MinVersion, "不应改变调用者基础传输的 TLS 下限")
	assert.Nil(t, base.TLSClientConfig.EncryptedClientHelloRejectionVerify)
}

// TestECHTransportComposesWithProxy 断言原语与代理可组合：
// 请求经代理发出，且 ECH 仍然生效。
func TestECHTransportComposesWithProxy(t *testing.T) {
	es := newECHTestServer(t, true)
	proxy := newECHTestProxy(t, es.addr)
	proxyURL, err := url.Parse(proxy.url)
	require.NoError(t, err)

	// 经代理时不能劫持拨号：拨号目标是代理地址，由代理建立到目标主机的隧道。
	// 因此这里用真实拨号，让请求按 example.com:443 发起 CONNECT。
	base := &http.Transport{
		TLSClientConfig: es.newServerTLS(),
		Proxy:           http.ProxyURL(proxyURL),
	}
	var seen echObserved
	observing(base, &seen)
	rt := NewECHTransport(base, WithECHConfigList(echTestServerConfigList(t, es)))

	resp, err := (&http.Client{Transport: rt}).Get("https://" + echTestRealHost + "/")
	require.NoError(t, err)
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	assert.NotZero(t, proxy.connects.Load(), "请求应经代理发出")
	assert.True(t, seen.accepted, "经代理时 ECH 仍应生效")
	assert.Equal(t, echTestPublicName, es.seenOuterSNI()[0])
}

// TestECHTransportNilBase 断言未提供基础传输时使用进程默认传输。
func TestECHTransportNilBase(t *testing.T) {
	assert.NotNil(t, NewECHTransport(nil))
}

// TestECHTransportDefaultPublicNameIsCloudflare 断言默认外层名是 Cloudflare 的公共名。
func TestECHTransportDefaultPublicNameIsCloudflare(t *testing.T) {
	rt := NewECHTransport(nil)
	tr, ok := rt.(*echTransport)
	require.True(t, ok)
	assert.Equal(t, "cloudflare-ech.com", tr.publicName)
	assert.Equal(t, ECHDefaultPublicName, tr.publicName)
}

// #endregion

// #region 测试支撑

// echTestServerConfigList 返回该服务端持有的配置所对应的 ECHConfigList。
func echTestServerConfigList(t *testing.T, es *echTestServer) []byte {
	t.Helper()
	return marshalECHConfigList(es.config)
}

// echTestForeignConfigList 返回一份格式合法但目标服务端无法解密的配置，
// 等价于「配置已轮换、客户端仍持旧配置」的情形。
//
// 公钥必须是对应 KEM 下的合法点，否则原语会在本地跳过它而发不出请求。
func echTestForeignConfigList(t *testing.T) []byte {
	t.Helper()
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	require.NoError(t, err)
	config, err := marshalECHConfig(30, key.PublicKey().Bytes(), echTestPublicName)
	require.NoError(t, err)
	return marshalECHConfigList(config)
}

// echObserved 记录客户端观测到的连接状态。
//
// 观测由测试自己的 VerifyConnection 完成：原语不应劫持调用者的回调，
// 因此这里验证的是「调用者的回调仍然生效」这一组合能力。
type echObserved struct {
	accepted bool
}

// observing 返回一份在握手后记录 ECH 是否被接受的 TLS 配置修改函数。
//
// 它用来在测试侧观测结果，而不是读取原语内部状态：原语不暴露内部字段，
// 调用者要观测 ECH 是否生效只能通过自己的 VerifyConnection。
func observing(rt *http.Transport, seen *echObserved) {
	base := rt
	if base.TLSClientConfig == nil {
		base.TLSClientConfig = new(tls.Config)
	}
	previous := base.TLSClientConfig.VerifyConnection
	base.TLSClientConfig.VerifyConnection = func(cs tls.ConnectionState) error {
		seen.accepted = cs.ECHAccepted
		if previous != nil {
			return previous(cs)
		}
		return nil
	}
}

// #endregion

// #region CONNECT 代理

// echTestProxy 是最小的本地 HTTP CONNECT 代理，用于断言经代理时 ECH 仍然生效。
type echTestProxy struct {
	srv       *http.Server
	url       string
	forwardTo string
	connects  atomic.Int64
}

// newECHTestProxy 启动代理，把所有隧道转发到 forwardTo。
func newECHTestProxy(t *testing.T, forwardTo string) *echTestProxy {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	p := &echTestProxy{forwardTo: forwardTo}
	p.srv = &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			http.Error(w, "仅支持 CONNECT", http.StatusMethodNotAllowed)
			return
		}
		p.connects.Add(1)
		upstream, err := net.Dial("tcp", p.forwardTo)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		hj, ok := w.(http.Hijacker)
		if !ok {
			upstream.Close()
			http.Error(w, "不支持 Hijack", http.StatusInternalServerError)
			return
		}
		clientConn, buf, err := hj.Hijack()
		if err != nil {
			upstream.Close()
			return
		}
		clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		// buf 中可能已缓存客户端在 CONNECT 之后立即发来的字节（ClientHello），
		// 必须先冲刷，否则会被丢弃。
		go func() {
			io.Copy(upstream, buf)
			upstream.Close()
		}()
		go func() {
			io.Copy(clientConn, upstream)
			clientConn.Close()
		}()
	})}
	go p.srv.Serve(ln)
	t.Cleanup(func() { p.srv.Close() })

	p.url = "http://" + ln.Addr().String()
	return p
}

// #endregion
