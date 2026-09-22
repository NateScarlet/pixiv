package connectivity

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeProber 用可替换的函数实现 Prober，记录每次调用，
// 使编排与判定逻辑的测试不依赖真实网络。
//
// 套件并行调度探测，因此调用记录需要互斥：无保护的 append 会让并发
// 调用互相覆盖，断言看到的调用清单随机缺项。
type fakeProber struct {
	resolve func(ctx context.Context, endpoint, host string, viaProxy bool) ([]net.IP, error)
	https   func(ctx context.Context, rawURL string, viaProxy bool) error
	ech     func(ctx context.Context, host string) error
	noSNI   func(ctx context.Context, host string) error

	mu    sync.Mutex
	calls []string
}

// recordCall 记下一次调用。
func (f *fakeProber) recordCall(format string, args ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fmt.Sprintf(format, args...))
}

// recordedCalls 返回已记录的调用清单副本。
func (f *fakeProber) recordedCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeProber) ProbeResolver(ctx context.Context, endpoint, host string, viaProxy bool) ([]net.IP, error) {
	f.recordCall("resolve:%v:%s:%s", viaProxy, endpoint, host)
	if f.resolve == nil {
		return nil, errors.New("未预期的 ProbeResolver 调用")
	}
	return f.resolve(ctx, endpoint, host, viaProxy)
}

func (f *fakeProber) ProbeHTTPS(ctx context.Context, rawURL string, viaProxy bool) error {
	f.recordCall("https:%v:%s", viaProxy, rawURL)
	if f.https == nil {
		return errors.New("未预期的 ProbeHTTPS 调用")
	}
	return f.https(ctx, rawURL, viaProxy)
}

func (f *fakeProber) ProbeECH(ctx context.Context, host string) error {
	f.recordCall("ech:%s", host)
	if f.ech == nil {
		return errors.New("未预期的 ProbeECH 调用")
	}
	return f.ech(ctx, host)
}

func (f *fakeProber) ProbeNoSNI(ctx context.Context, host string) error {
	f.recordCall("nosni:%s", host)
	if f.noSNI == nil {
		return errors.New("未预期的 ProbeNoSNI 调用")
	}
	return f.noSNI(ctx, host)
}

// testEnv 构造测试环境：withProxy 决定是否配置了 HTTPS_PROXY。
func testEnv(withProxy bool) Environment {
	env := Environment{
		GoVersion:                 "go1.26.0",
		ResolverEndpoint:          "https://1.1.1.1/dns-query",
		ResolverEndpointIsDefault: true,
	}
	if withProxy {
		env.HTTPSProxyRaw = "http://127.0.0.1:7890"
		env.ProxyURL = &url.URL{Scheme: "http", Host: "127.0.0.1:7890"}
	}
	return env
}

// runSuite 用 fake 探测器运行默认套件。
func runSuite(t *testing.T, env Environment, p Prober) Report {
	t.Helper()
	return Suite{}.Run(context.Background(), env, p)
}

// TestSuiteZeroValueUsesDefaultHosts 断言零值 Suite 使用默认主机清单：
// 漏传字段的调用者得到的是有意义的探测，而不是空报告。
func TestSuiteZeroValueUsesDefaultHosts(t *testing.T) {
	p := &fakeProber{
		resolve: func(context.Context, string, string, bool) ([]net.IP, error) {
			return []net.IP{net.ParseIP("210.140.139.129")}, nil
		},
		ech:   func(context.Context, string) error { return nil },
		noSNI: func(context.Context, string) error { return nil },
		https: func(context.Context, string, bool) error { return nil },
	}
	rep := runSuite(t, testEnv(false), p)

	assert.Contains(t, p.recordedCalls(), "ech:www.pixiv.net")
	assert.Contains(t, p.recordedCalls(), "ech:app-api.pixiv.net")
	assert.Contains(t, p.recordedCalls(), "nosni:i.pximg.net")
	assert.Contains(t, p.recordedCalls(), "https:false:https://www.pixiv.net/")
	assert.Contains(t, p.recordedCalls(), "https:false:https://i.pximg.net/")
	assert.NotEmpty(t, rep.Checks)
}

// TestSuiteSkipsProxyProbeForNonHTTPEndpoint 断言解析方式不经 HTTP 时
// 不发起经代理的解析探测：明文 DNS 走 UDP、系统解析走平台 API，都没有
// 可经代理的路径，发起这种探测只会产出必然失败的噪音。
func TestSuiteSkipsProxyProbeForNonHTTPEndpoint(t *testing.T) {
	p := &fakeProber{
		resolve: func(context.Context, string, string, bool) ([]net.IP, error) {
			return []net.IP{net.ParseIP("210.140.139.129")}, nil
		},
		ech:   func(context.Context, string) error { return nil },
		noSNI: func(context.Context, string) error { return nil },
		https: func(context.Context, string, bool) error { return nil },
	}
	env := testEnv(true)
	env.ResolverEndpoint = "dns://1.1.1.1"
	runSuite(t, env, p)

	for _, c := range p.recordedCalls() {
		if strings.HasPrefix(c, "resolve:") {
			assert.NotContains(t, c, ":true:", "不经 HTTP 的解析端点不应探测代理路径: %s", c)
		}
	}
}

// TestSuiteProbesProxyOnlyWhenConfigured 断言未配置代理时不发起任何经代理的探测：
// 那些探测必然失败，执行它们只会产出噪音。
func TestSuiteProbesProxyOnlyWhenConfigured(t *testing.T) {
	p := &fakeProber{
		resolve: func(context.Context, string, string, bool) ([]net.IP, error) {
			return []net.IP{net.ParseIP("210.140.139.129")}, nil
		},
		ech:   func(context.Context, string) error { return nil },
		noSNI: func(context.Context, string) error { return nil },
		https: func(_ context.Context, rawURL string, viaProxy bool) error {
			if viaProxy {
				return errors.New("不应在未配置代理时探测代理路径")
			}
			return nil
		},
	}
	runSuite(t, testEnv(false), p)

	for _, c := range p.recordedCalls() {
		assert.NotContains(t, c, ":true:", "未配置代理时不应有经代理的探测: %s", c)
	}
}

// TestSuiteProbesProxyWhenConfigured 断言配置了 HTTPS_PROXY 时，
// 解析端点与各主机都会额外探测经代理的路径——这是「数据是否需要代理」结论的依据。
func TestSuiteProbesProxyWhenConfigured(t *testing.T) {
	p := &fakeProber{
		resolve: func(context.Context, string, string, bool) ([]net.IP, error) {
			return []net.IP{net.ParseIP("210.140.139.129")}, nil
		},
		ech:   func(context.Context, string) error { return nil },
		noSNI: func(context.Context, string) error { return nil },
		https: func(context.Context, string, bool) error { return nil },
	}
	runSuite(t, testEnv(true), p)

	assert.Contains(t, p.recordedCalls(), "resolve:true:https://1.1.1.1/dns-query:i.pximg.net")
	assert.Contains(t, p.recordedCalls(), "https:true:https://www.pixiv.net/")
	assert.Contains(t, p.recordedCalls(), "https:true:https://i.pximg.net/")
}

// TestSuiteResolverQueriedForImageHost 断言解析探测查询的是图片主机：
// 它同时是后续无 SNI 直连要解析的名字，查询它即可验证端点可用。
func TestSuiteResolverQueriedForImageHost(t *testing.T) {
	p := &fakeProber{
		resolve: func(_ context.Context, _ string, host string, _ bool) ([]net.IP, error) {
			require.Equal(t, "i.pximg.net", host)
			return []net.IP{net.ParseIP("210.140.139.129")}, nil
		},
		ech:   func(context.Context, string) error { return nil },
		noSNI: func(context.Context, string) error { return nil },
		https: func(context.Context, string, bool) error { return nil },
	}
	runSuite(t, testEnv(false), p)

	assert.Contains(t, p.recordedCalls(), "resolve:false:https://1.1.1.1/dns-query:i.pximg.net")
}

// TestVerdictFullyDirect 断言 ECH 与无 SNI 直连都成功、常规直连被封锁时，
// 结论仍是「可完全直连」：库的默认传输正是靠这两种特殊方式工作的，
// 常规直连失败不妨碍 AutoTransport 可用。
func TestVerdictFullyDirect(t *testing.T) {
	p := &fakeProber{
		resolve: func(_ context.Context, _ string, _ string, viaProxy bool) ([]net.IP, error) {
			if viaProxy {
				return nil, errors.New("未配置代理")
			}
			return []net.IP{net.ParseIP("210.140.139.129")}, nil
		},
		ech:   func(context.Context, string) error { return nil },
		noSNI: func(context.Context, string) error { return nil },
		https: func(_ context.Context, _ string, viaProxy bool) error {
			if viaProxy {
				return errors.New("未配置代理")
			}
			return errors.New("SNI 阻断")
		},
	}
	rep := runSuite(t, testEnv(false), p)

	v := rep.Verdict()
	assert.Equal(t, LevelDirect, v.Level)
	assert.Contains(t, v.Text, "直连")
	assert.Contains(t, rep.API.DirectWays, "ECH")
	assert.Contains(t, rep.Image.DirectWays, "无 SNI")
	assert.False(t, rep.API.ViaProxy, "未配置代理时不应报告代理路径")
}

// TestVerdictResolverNeedsProxy 断言配置了代理、解析直连失败而经代理成功时，
// 结论明确指出解析需要经过代理，且运行时默认配置已自动满足（解析查询经
// http.DefaultClient 发出，遵循 HTTPS_PROXY）。
func TestVerdictResolverNeedsProxy(t *testing.T) {
	p := &fakeProber{
		resolve: func(_ context.Context, _ string, _ string, viaProxy bool) ([]net.IP, error) {
			if viaProxy {
				return []net.IP{net.ParseIP("210.140.139.129")}, nil
			}
			return nil, errors.New("connection refused")
		},
		ech:   func(context.Context, string) error { return nil },
		noSNI: func(context.Context, string) error { return nil },
		https: func(_ context.Context, _ string, viaProxy bool) error {
			if viaProxy {
				return nil
			}
			return errors.New("SNI 阻断")
		},
	}
	rep := runSuite(t, testEnv(true), p)

	v := rep.Verdict()
	assert.Equal(t, LevelDirect, v.Level, "数据路径可直连，解析的代理需求不改变直连结论")
	assert.Contains(t, v.Text, "解析")
	assert.Contains(t, v.Text, "代理")
	assert.True(t, rep.Resolver.NeedsProxy())
}

// TestVerdictResolverUsableWithoutProxy 断言配置了代理但解析直连成功时，
// 结论明确说明解析无需经过代理——这是用户要求的三项判定之一。
func TestVerdictResolverUsableWithoutProxy(t *testing.T) {
	p := &fakeProber{
		resolve: func(context.Context, string, string, bool) ([]net.IP, error) {
			return []net.IP{net.ParseIP("210.140.139.129")}, nil
		},
		ech:   func(context.Context, string) error { return nil },
		noSNI: func(context.Context, string) error { return nil },
		https: func(context.Context, string, bool) error { return nil },
	}
	rep := runSuite(t, testEnv(true), p)

	v := rep.Verdict()
	assert.Equal(t, LevelDirect, v.Level)
	assert.False(t, rep.Resolver.NeedsProxy())
	assert.Contains(t, v.Text, "解析")
}

// TestVerdictDataNeedsProxy 断言直连全失败、经代理可达时，
// 结论是「数据传输需要经过代理」，且 AutoTransport 仍可用。
func TestVerdictDataNeedsProxy(t *testing.T) {
	p := &fakeProber{
		resolve: func(_ context.Context, _ string, _ string, viaProxy bool) ([]net.IP, error) {
			if viaProxy {
				return []net.IP{net.ParseIP("210.140.139.129")}, nil
			}
			return nil, errors.New("connection refused")
		},
		ech:   func(context.Context, string) error { return errors.New("ECH 自举失败") },
		noSNI: func(context.Context, string) error { return errors.New("无 SNI 直连失败") },
		https: func(_ context.Context, _ string, viaProxy bool) error {
			if viaProxy {
				return nil
			}
			return errors.New("连接被重置")
		},
	}
	rep := runSuite(t, testEnv(true), p)

	v := rep.Verdict()
	assert.Equal(t, LevelProxyRequired, v.Level)
	assert.Contains(t, v.Text, "代理")
	assert.False(t, rep.API.Direct)
	assert.True(t, rep.API.ViaProxy)
	assert.True(t, rep.AutoTransportUsable())
}

// TestVerdictPartial 断言仅一类数据可直连时结论为「部分直连」，
// 并指明需要代理的是哪一类。
func TestVerdictPartial(t *testing.T) {
	p := &fakeProber{
		resolve: func(_ context.Context, _ string, _ string, viaProxy bool) ([]net.IP, error) {
			if viaProxy {
				return []net.IP{net.ParseIP("210.140.139.129")}, nil
			}
			return nil, errors.New("connection refused")
		},
		ech:   func(context.Context, string) error { return nil },
		noSNI: func(context.Context, string) error { return errors.New("无 SNI 直连失败") },
		https: func(_ context.Context, _ string, viaProxy bool) error {
			if viaProxy {
				return nil
			}
			return errors.New("连接被重置")
		},
	}
	rep := runSuite(t, testEnv(true), p)

	v := rep.Verdict()
	assert.Equal(t, LevelPartial, v.Level)
	assert.Contains(t, v.Text, "图片")
	assert.Contains(t, v.Text, "代理")
	assert.True(t, rep.API.Direct)
	assert.True(t, rep.Image.ViaProxy)
}

// TestVerdictUnavailableWithoutProxy 断言直连全失败且未配置代理时，
// 结论是「不可用」：AutoTransport 没有任何可用路径。
func TestVerdictUnavailableWithoutProxy(t *testing.T) {
	p := &fakeProber{
		resolve: func(context.Context, string, string, bool) ([]net.IP, error) {
			return nil, errors.New("connection refused")
		},
		ech:   func(context.Context, string) error { return errors.New("ECH 自举失败") },
		noSNI: func(context.Context, string) error { return errors.New("无 SNI 直连失败") },
		https: func(context.Context, string, bool) error { return errors.New("连接被重置") },
	}
	rep := runSuite(t, testEnv(false), p)

	v := rep.Verdict()
	assert.Equal(t, LevelUnavailable, v.Level)
	assert.False(t, rep.AutoTransportUsable())
}

// TestVerdictUnavailableWithProxy 断言代理路径也全失败时结论仍为「不可用」，
// 且报告中包含代理路径失败的探测记录。
func TestVerdictUnavailableWithProxy(t *testing.T) {
	p := &fakeProber{
		resolve: func(context.Context, string, string, bool) ([]net.IP, error) {
			return nil, errors.New("connection refused")
		},
		ech:   func(context.Context, string) error { return errors.New("ECH 自举失败") },
		noSNI: func(context.Context, string) error { return errors.New("无 SNI 直连失败") },
		https: func(context.Context, string, bool) error { return errors.New("连接被重置") },
	}
	rep := runSuite(t, testEnv(true), p)

	v := rep.Verdict()
	assert.Equal(t, LevelUnavailable, v.Level)
	assert.False(t, rep.AutoTransportUsable())
	assert.NotEmpty(t, rep.Checks)
}

// TestSuiteProbesConcurrently 断言各探测并行执行：全部探测都到达屏障后才放行，
// 若串行执行则永远到不齐、只能在套件超时后失败。
func TestSuiteProbesConcurrently(t *testing.T) {
	// 无代理环境的任务数：1 次解析 + 2 台 API 主机 × 2 方式 + 1 台图片主机 × 2 方式。
	const taskCount = 7

	arrived := make(chan struct{}, taskCount)
	allArrived := make(chan struct{})
	go func() {
		for i := 0; i < taskCount; i++ {
			<-arrived
		}
		close(allArrived)
	}()
	barrier := func(ctx context.Context) error {
		arrived <- struct{}{}
		select {
		case <-allArrived:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	p := &fakeProber{
		resolve: func(_ context.Context, _, _ string, _ bool) ([]net.IP, error) {
			return []net.IP{net.ParseIP("210.140.139.129")}, barrier(context.Background())
		},
		ech: func(ctx context.Context, _ string) error { return barrier(ctx) },
		noSNI: func(ctx context.Context, _ string) error {
			return barrier(ctx)
		},
		https: func(ctx context.Context, _ string, _ bool) error {
			return barrier(ctx)
		},
	}
	suite := Suite{Timeout: 500 * time.Millisecond}
	rep := suite.Run(context.Background(), testEnv(false), p)

	for _, c := range rep.Checks {
		assert.NoError(t, c.Err, "全部探测应同时运行并互相放行: %s", c.Name)
	}
}

// TestSuiteProbeTimeoutLimitsEachProbe 断言单个探测超过套件超时即失败，
// 且错误说明是探测超时而不是含糊的底层错误。
func TestSuiteProbeTimeoutLimitsEachProbe(t *testing.T) {
	p := &fakeProber{
		resolve: func(context.Context, string, string, bool) ([]net.IP, error) {
			return []net.IP{net.ParseIP("210.140.139.129")}, nil
		},
		ech: func(context.Context, string) error {
			select {
			case <-time.After(time.Second):
				return nil
			case <-context.Background().Done():
				return context.Background().Err()
			}
		},
		noSNI: func(context.Context, string) error { return nil },
		https: func(context.Context, string, bool) error { return nil },
	}
	suite := Suite{Timeout: 50 * time.Millisecond}
	rep := suite.Run(context.Background(), testEnv(false), p)

	var found bool
	for _, c := range rep.Checks {
		if c.Name == "API 主机 ECH 直连" && c.Detail == "www.pixiv.net" {
			found = true
			require.Error(t, c.Err)
			assert.Contains(t, c.Err.Error(), "探测超时")
		}
	}
	assert.True(t, found, "应包含 www.pixiv.net 的 ECH 探测记录")
}
