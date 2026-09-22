package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/NateScarlet/pixiv/pkg/client/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// spyTransport 记录经过它的请求，用于在不触网的前提下断言请求的可观测特征。
//
// next 为空时用固定的空响应对答，用例可只看请求；非空时把请求转给它，
// 从而让同一用例既能断言请求特征、又能断言真实响应内容。
type spyTransport struct {
	mu       sync.Mutex
	requests []*http.Request
	next     http.RoundTripper
}

func (t *spyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	t.requests = append(t.requests, req)
	t.mu.Unlock()
	if t.next != nil {
		return t.next.RoundTrip(req)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("{}")),
		Request:    req,
	}, nil
}

func (t *spyTransport) count() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.requests)
}

func (t *spyTransport) last() *http.Request {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.requests) == 0 {
		return nil
	}
	return t.requests[len(t.requests)-1]
}

// useDefaultTransport 在用例期间替换模块级默认传输。
func useDefaultTransport(t *testing.T, rt http.RoundTripper) {
	t.Helper()
	previous := DefaultTransport
	DefaultTransport = rt
	t.Cleanup(func() { DefaultTransport = previous })
}

// requestHeaders 发送一个请求并返回该请求被观测到的请求头。
func requestHeaders(t *testing.T, c *Client, spy *spyTransport, rawURL string) http.Header {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	require.NoError(t, err)
	resp, err := c.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 1, spy.count())
	return spy.last().Header
}

// TestDefaultHasSameSemanticsAsNew 断言包级默认客户端与 New() 语义一致：
// 两者都由同一装配路径产生，默认值也一样。
func TestDefaultHasSameSemanticsAsNew(t *testing.T) {
	require.NotNil(t, Default)
	require.NotNil(t, Default.Transport)

	// 同一装配路径：服务地址、传输的具体类型都一致。
	fresh := New()
	assert.Equal(t, fresh.serverURL, Default.serverURL)
	assert.Equal(t, fmt.Sprintf("%T", fresh.Transport), fmt.Sprintf("%T", Default.Transport))

	want := New().EndpointURL("/x", nil)
	got := Default.EndpointURL("/x", nil)
	assert.Equal(t, want.String(), got.String())
}

// TestZeroClientIsSafe 断言零值 Client 是安全值：可作为普通 HTTP 客户端使用，
// 也可被值拷贝；EndpointURL 按约定回落默认服务地址。
func TestZeroClientIsSafe(t *testing.T) {
	var c Client

	u := c.EndpointURL("/x", nil)
	assert.Equal(t, "https://www.pixiv.net/x", u.String())

	// 零值的传输为空，由 net/http 回落到标准传输。
	assert.Nil(t, c.Transport)

	// 零值可被值拷贝且不触发同步状态问题。
	copied := c
	assert.Equal(t, c.serverURL, copied.serverURL)
}

// TestNewExplicitEmptyPanics 断言显式设置为零值时快速失败：
// 未设置由默认值填充，显式设置为空不是任何环境的描述。
func TestNewExplicitEmptyPanics(t *testing.T) {
	assert.PanicsWithValue(t,
		"pixiv: client: WithServerURL 的值为空: 未设置时省略该选项即可使用默认值",
		func() { New(WithServerURL("")) })
	assert.PanicsWithValue(t,
		"pixiv: client: WithPHPSESSID 的值为空: 未设置时省略该选项即可，显式设置为空不被允许",
		func() { New(WithPHPSESSID("")) })
	assert.PanicsWithValue(t,
		"pixiv: client: WithTransport 的值为空: 未设置时省略该选项即可使用 DefaultTransport",
		func() { New(WithTransport(nil)) })
	assert.PanicsWithValue(t,
		"pixiv: client: DefaultTransport 被置空而调用者未提供传输: 替换 DefaultTransport 时不能设为 nil",
		func() {
			useDefaultTransport(t, nil)
			New()
		})
}

// TestNewDoesNotSendRequest 断言构造客户端不产生任何网络请求。
func TestNewDoesNotSendRequest(t *testing.T) {
	spy := &spyTransport{}
	useDefaultTransport(t, spy)

	_ = New()
	_ = New(WithUserAgent("ua"), WithPHPSESSID("session"), WithServerURL("https://example.com"))

	assert.Equal(t, 0, spy.count(), "New 不应发起任何请求")
}

// TestNewUsesDefaultTransport 断言未显式传传输时使用 DefaultTransport。
func TestNewUsesDefaultTransport(t *testing.T) {
	spy := &spyTransport{}
	useDefaultTransport(t, spy)

	c := New()
	resp, err := c.Get("https://example.com/")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, 1, spy.count())
}

// TestNewExplicitTransportWins 断言显式传入的传输优先于 DefaultTransport。
func TestNewExplicitTransportWins(t *testing.T) {
	defaultSpy := &spyTransport{}
	explicitSpy := &spyTransport{}
	useDefaultTransport(t, defaultSpy)

	c := New(WithTransport(explicitSpy))
	resp, err := c.Get("https://example.com/")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, 1, explicitSpy.count())
	assert.Equal(t, 0, defaultSpy.count(), "显式传输生效时不应使用 DefaultTransport")
}

// TestEndpointURL 断言服务地址未设置时填默认值、显式设置胜出、
// 同一项多次设置以最后一个为准。
func TestEndpointURL(t *testing.T) {
	for _, tt := range []struct {
		name    string
		options []Option
		want    string
	}{
		{"未设置时填默认值", nil, "https://www.pixiv.net/ajax/illust/1"},
		{"显式设置胜出", []Option{WithServerURL("https://mirror.example.com")}, "https://mirror.example.com/ajax/illust/1"},
		{"多次设置以最后一个为准", []Option{
			WithServerURL("https://first.example.com"),
			WithServerURL("https://second.example.com"),
		}, "https://second.example.com/ajax/illust/1"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := New(tt.options...)
			assert.Equal(t, tt.want, c.EndpointURL("/ajax/illust/1", nil).String())
		})
	}
}

// TestNewInvalidServerURLPanics 断言无法解析的服务地址在装配期快速失败，
// 而不是等首次请求才报错。
func TestNewInvalidServerURLPanics(t *testing.T) {
	assert.PanicsWithValue(t,
		`pixiv: client: WithServerURL 的值 "://invalid" 无法解析: parse "://invalid": missing protocol scheme`,
		func() { New(WithServerURL("://invalid")) })
}

// TestGetBodyRequestFallsBack 断言带可重建 body 的请求在首选方式失败时
// 以重建后的 body 尝试其余方式，全部失败时聚合两种方式的错误。
func TestGetBodyRequestFallsBack(t *testing.T) {
	var baseSeenBodies []string
	base := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(req.Body)
		baseSeenBodies = append(baseSeenBodies, string(b))
		return nil, errors.New("stub: 不可用")
	})
	rt := &AutoTransport{Base: base}
	rt.once.Do(func() {
		rt.base = base
		rt.routed = newRoutedTransportWithRoutes(base, map[string]route{
			"i.pximg.net": {rt: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				return nil, errors.New("stub: 特殊方式不可用")
			})},
		})
	})

	req, err := http.NewRequest(http.MethodPost, "https://i.pximg.net/upload", strings.NewReader("payload"))
	require.NoError(t, err)
	_, err = rt.RoundTrip(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stub: 特殊方式不可用")
	assert.Contains(t, err.Error(), "stub: 不可用")
	// 首选方式因请求未发出而未消费 body，重试时应以完整 body 发出。
	assert.Equal(t, []string{"payload"}, baseSeenBodies)
}

// TestUserAgent 断言未设置时填默认值、显式设置胜出、多次设置以最后一个为准。
func TestUserAgent(t *testing.T) {
	t.Setenv("PIXIV_USER_AGENT", "")
	spy := &spyTransport{}
	c := New(WithTransport(spy))
	header := requestHeaders(t, c, spy, "https://example.com/")
	assert.Equal(t, defaultUserAgent, header.Get("User-Agent"))

	spy = &spyTransport{}
	c = New(WithTransport(spy), WithUserAgent("first"), WithUserAgent("last"))
	header = requestHeaders(t, c, spy, "https://example.com/")
	assert.Equal(t, "last", header.Get("User-Agent"))
}

// TestUserAgentSeededFromEnv 断言环境变量作为默认值的播种来源仍然有效。
func TestUserAgentSeededFromEnv(t *testing.T) {
	t.Setenv("PIXIV_USER_AGENT", "env-agent")
	spy := &spyTransport{}
	c := New(WithTransport(spy))
	header := requestHeaders(t, c, spy, "https://example.com/")
	assert.Equal(t, "env-agent", header.Get("User-Agent"))
}

// TestUserAgentExplicitEmpty 断言显式设置为零值时不使用默认值，
// 即选项区分「未设置」与「已设置为零值」。
func TestUserAgentExplicitEmpty(t *testing.T) {
	t.Setenv("PIXIV_USER_AGENT", "env-agent")
	spy := &spyTransport{}
	c := New(WithTransport(spy), WithUserAgent(""))
	header := requestHeaders(t, c, spy, "https://example.com/")
	assert.Empty(t, header.Get("User-Agent"), "显式设置为空表示不使用默认 User-Agent")
}

// TestPHPSESSID 断言选项提供的凭据自动作用于正确的域：
// 发送给 pixiv 的主机（含子域），不发送给无关主机。
func TestPHPSESSID(t *testing.T) {
	t.Setenv("PIXIV_PHPSESSID", "")
	spy := &spyTransport{}
	c := New(WithTransport(spy), WithPHPSESSID("session-value"))

	for _, tt := range []struct {
		name string
		url  string
		want bool
	}{
		{"主站", "https://www.pixiv.net/", true},
		{"同域子站", "https://app-api.pixiv.net/", true},
		{"其他域不携带", "https://i.pximg.net/x.png", false},
		{"无关主机不携带", "https://example.com/", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			spy.mu.Lock()
			spy.requests = nil
			spy.mu.Unlock()

			resp, err := c.Get(tt.url)
			require.NoError(t, err)
			defer resp.Body.Close()

			cookies := spy.last().Cookies()
			if !tt.want {
				assert.Empty(t, cookies, "凭据不应发送给无关主机")
				return
			}
			require.Len(t, cookies, 1)
			assert.Equal(t, "PHPSESSID", cookies[0].Name)
			assert.Equal(t, "session-value", cookies[0].Value)
		})
	}
}

// TestPHPSESSIDSeededFromEnv 断言环境变量作为默认值的播种来源仍然有效。
func TestPHPSESSIDSeededFromEnv(t *testing.T) {
	t.Setenv("PIXIV_PHPSESSID", "env-session")
	spy := &spyTransport{}
	c := New(WithTransport(spy))
	req, err := http.NewRequest(http.MethodGet, "https://www.pixiv.net/", nil)
	require.NoError(t, err)
	resp, err := c.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	cookies := spy.last().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, "env-session", cookies[0].Value)
}

// TestClientConcurrentUseAndCopy 断言同一个 Client 可被多个 goroutine 并发使用，
// 且值拷贝出的 Client 也能正常工作（spec：并发与拷贝安全）。
func TestClientConcurrentUseAndCopy(t *testing.T) {
	spy := &spyTransport{}
	c := New(WithTransport(spy), WithUserAgent("ua"))
	copied := *c

	const goroutines = 8
	const requestsPerGoroutine = 16
	var wg sync.WaitGroup
	errCh := make(chan error, goroutines*requestsPerGoroutine)
	for i := 0; i < goroutines; i++ {
		target := c
		if i%2 == 0 {
			// 一半 goroutine 使用值拷贝出的客户端。
			target = &copied
		}
		wg.Add(1)
		go func(c *Client) {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine; j++ {
				resp, err := c.Get("https://example.com/")
				if err != nil {
					errCh <- err
					continue
				}
				resp.Body.Close()
			}
		}(target)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
	assert.Equal(t, goroutines*requestsPerGoroutine, spy.count())
}

// TestDNSQueryURLDeclaresWireFormat 断言查询方式由 PIXIV_DNS_QUERY_URL 的
// fragment 声明，无需单独的配置项。
//
// 只支持 RFC 8484 二进制报文的服务端（例如 dnscrypt-proxy 的本地 DoH
// 服务端）在收到 JSON 接口的查询时以 400 拒绝，因此需要能切换到 JSON
// 之外的写法；默认即二进制，符合标准对实现的要求。
func TestDNSQueryURLDeclaresWireFormat(t *testing.T) {
	var gotQuery url.Values
	var gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		gotQuery = req.URL.Query()
		gotAccept = req.Header.Get("Accept")
		// 只认 RFC 8484：没有 dns 参数即拒绝。
		if req.URL.Query().Get("dns") == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/dns-message")
		// ID 回显 + 一条 A 记录的最小应答。
		_, _ = w.Write([]byte{
			0x00, 0x00, 0x81, 0x80, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00,
			1, 'i', 5, 'p', 'x', 'i', 'm', 'g', 3, 'n', 'e', 't', 0,
			0x00, 0x01, 0x00, 0x01,
			0xc0, 0x0c, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x3c, 0x00, 0x04,
			210, 140, 139, 129,
		})
	}))
	t.Cleanup(srv.Close)

	t.Run("默认二进制", func(t *testing.T) {
		t.Setenv("PIXIV_DNS_QUERY_URL", srv.URL)
		ips, err := defaultDNSResolver().Resolve(context.Background(), "i.pximg.net")
		require.NoError(t, err)
		assert.Equal(t, "application/dns-message", gotAccept)
		assert.Empty(t, gotQuery.Get("name"), "二进制方式不应发 name 参数")
		require.Len(t, ips, 1)
		assert.Equal(t, "210.140.139.129", ips[0].String())
	})

	t.Run("fragment 声明不进入请求", func(t *testing.T) {
		t.Setenv("PIXIV_DNS_QUERY_URL", srv.URL+"#type=message")
		_, err := defaultDNSResolver().Resolve(context.Background(), "i.pximg.net")
		require.NoError(t, err)
		assert.Empty(t, gotQuery.Get("name"))
	})

	t.Run("声明 json 时走 JSON 接口", func(t *testing.T) {
		t.Setenv("PIXIV_DNS_QUERY_URL", srv.URL+"#type=json")
		// 该服务端只认二进制，因此走 JSON 必然被拒绝——这正说明它确实是 JSON 方式。
		_, err := defaultDNSResolver().Resolve(context.Background(), "i.pximg.net")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "status 400")
	})
}

// TestDNSQueryURLInvalidFragmentPanics 断言 fragment 取值非法时快速失败：
// 静默回落会得到「端点拒绝查询」的误导性错误，而真实原因是 URL 写错了。
func TestDNSQueryURLInvalidFragmentPanics(t *testing.T) {
	t.Setenv("PIXIV_DNS_QUERY_URL", "https://doh.example/dns-query#type=binary")
	assert.PanicsWithValue(t,
		`pixiv: dns: DoH 端点地址 "https://doh.example/dns-query#type=binary": type 的值 "binary" 无效: 可用值为 json、message`,
		func() { defaultDNSResolver() })
}

// TestDNSQueryURLSelectsResolver 断言 PIXIV_DNS_QUERY_URL 的 scheme 选择解析方式。
//
// 本用例在环境变量这一接缝上钉住 scheme 分派：dns: 走系统解析。若该值仍被
// 交给 DoH 解析器，会因缺少主机名而在构造期 panic，因此「能解析出 localhost」
// 足以说明分派确实发生在 scheme 上。
func TestDNSQueryURLSelectsResolver(t *testing.T) {
	t.Setenv("PIXIV_DNS_QUERY_URL", "dns:")
	ips, err := defaultDNSResolver().Resolve(context.Background(), "localhost")
	require.NoError(t, err)
	assert.NotEmpty(t, ips)
}

// TestDNSQueryURLInvalidSchemePanics 断言端点写法非法时快速失败，
// 而不是回落到默认值：静默回落会让写错配置的人以为设置在生效。
func TestDNSQueryURLInvalidSchemePanics(t *testing.T) {
	t.Setenv("PIXIV_DNS_QUERY_URL", "tls://1.1.1.1")
	assert.Panics(t, func() { defaultDNSResolver() })
}

// resolverFunc 便于在测试中构造解析器。
type resolverFunc func(ctx context.Context, host string) ([]net.IP, error)

func (f resolverFunc) Resolve(ctx context.Context, host string) ([]net.IP, error) {
	return f(ctx, host)
}

var _ dns.Resolver = resolverFunc(nil)

// failingDialTransport 提供拨号能力但拨号必然失败，用于在离线条件下
// 驱动本库自带的连接能力（它会先解析再拨号）。
func failingDialTransport() *http.Transport {
	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return nil, errors.New("stub: 不真正建立连接")
		},
	}
}

// TestDNSResolverOptionIsUsed 断言注入的解析器被本库自带的连接能力
// 用于解析需要特殊方式的主机，且不会为其他主机发起解析。
func TestDNSResolverOptionIsUsed(t *testing.T) {
	var mu sync.Mutex
	var resolved []string
	record := func(ctx context.Context, host string) ([]net.IP, error) {
		mu.Lock()
		resolved = append(resolved, host)
		mu.Unlock()
		return nil, errors.New("stub: 不真正解析")
	}

	useDefaultTransport(t, &AutoTransport{Base: failingDialTransport()})
	c := New(WithDNSResolver(resolverFunc(record)))

	// 需要特殊方式的主机：解析失败导致请求失败，错误应呈现给调用者。
	_, err := c.Get("https://i.pximg.net/x.png")
	require.Error(t, err)

	mu.Lock()
	assert.Contains(t, resolved, "i.pximg.net")
	resolved = nil
	mu.Unlock()

	// 普通主机不经过本库自带的连接能力，因此不应触发解析。
	_, _ = c.Get("https://example.com/")
	mu.Lock()
	assert.Empty(t, resolved)
	mu.Unlock()
}

// TestDNSResolverExplicitNil 断言显式传入 nil 表示使用系统解析，
// 此时本库自带的连接能力不再自行解析。
func TestDNSResolverExplicitNil(t *testing.T) {
	useDefaultTransport(t, &AutoTransport{Base: failingDialTransport()})
	c := New(WithDNSResolver(nil))
	_, err := c.Get("https://i.pximg.net/x.png")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "解析主机", "显式传入 nil 时不应由库自行解析")
}

// TestDNSResolverUsedByStandalonePrimitive 断言单独使用传输原语时，
// 注入的解析器同样被用于解析拨号目标（此场景下没有主机清单可依据）。
func TestDNSResolverUsedByStandalonePrimitive(t *testing.T) {
	var mu sync.Mutex
	var resolved []string
	record := func(ctx context.Context, host string) ([]net.IP, error) {
		mu.Lock()
		resolved = append(resolved, host)
		mu.Unlock()
		return nil, errors.New("stub: 不真正解析")
	}

	c := New(
		WithTransport(NewNoSNITransport(failingDialTransport())),
		WithDNSResolver(resolverFunc(record)),
	)
	_, err := c.Get("https://i.pximg.net/x.png")
	require.Error(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Contains(t, resolved, "i.pximg.net", "单独使用的原语也应使用注入的解析器")
}

// TestDNSResolverSkipsIPLiteral 断言拨号目标是 IP 字面量时不触发解析：
// 例如经代理时拨号的是代理地址。
func TestDNSResolverSkipsIPLiteral(t *testing.T) {
	var mu sync.Mutex
	var resolved []string
	record := func(ctx context.Context, host string) ([]net.IP, error) {
		mu.Lock()
		resolved = append(resolved, host)
		mu.Unlock()
		return nil, errors.New("stub: 不真正解析")
	}

	c := New(
		WithTransport(NewNoSNITransport(failingDialTransport())),
		WithDNSResolver(resolverFunc(record)),
	)
	_, err := c.Get("https://127.0.0.1:8443/x.png")
	require.Error(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Empty(t, resolved, "IP 字面量不需要解析")
}
