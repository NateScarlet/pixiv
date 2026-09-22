package connectivity

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/NateScarlet/pixiv/pkg/client/dns"
)

// apiHosts 是要探测的 API 类主机（Cloudflare 托管，走 ECH 直连）。
//
// 只含 www.pixiv.net：它是库实际发 API 请求的默认主机（client.defaultServerURL）。
// app-api.pixiv.net 虽与 www 同属 API 路由表，但库从不向它发请求，纳入探测会
// 掩盖 www 的失败，因此不作为 API 类连通性的判定对象。
var apiHosts = []string{"www.pixiv.net"}

// imageHosts 是要探测的图片类主机（走无 SNI 直连）。
var imageHosts = []string{"i.pximg.net"}

// DefaultTimeout 是单个探测的超时：连不上的地址再等也不会改变结论
// （超过这个时长即使用户侧最终连上，运行时的可用性也不可靠）。
const DefaultTimeout = 5 * time.Second

// Hosts 是按类别分组的主机清单。
type Hosts struct {
	API   []string
	Image []string
}

// Suite 执行默认的探测编排，产出报告。
//
// Hosts 为空时使用内置主机清单（与库的路由传输一致）；
// Timeout 为零值时使用 [DefaultTimeout]；零值 Suite 可直接使用。
type Suite struct {
	// Hosts 覆盖默认主机清单，供测试替换；为空表示使用默认清单。
	Hosts Hosts
	// Timeout 是单个探测的超时；全部探测并行执行，单项超时不影响其他项。
	Timeout time.Duration
}

// probeTask 是一次探测的执行单元：名称、明细与探测函数分离，
// 由套件统一并行调度并汇总为 Check 记录。
type probeTask struct {
	name   string
	detail string
	run    func(ctx context.Context) error
}

// recordDirectWay 把一种直连方式记入主机报告，同种方式（多台主机共享）
// 只记录一次，避免结论句出现「ECH、ECH」。
func recordDirectWay(h *HostReport, way string) {
	for _, w := range h.DirectWays {
		if w == way {
			return
		}
	}
	h.DirectWays = append(h.DirectWays, way)
}

// Run 执行全部探测并汇总报告。
//
// 探测相互独立、并行执行，单项失败（包括超时）不中止套件——诊断需要的
// 是每项的结果，而不是第一个失败；总耗时由最慢的一项决定，受 Timeout 约束。
func (s Suite) Run(ctx context.Context, env Environment, p Prober) Report {
	hosts := s.Hosts
	if len(hosts.API) == 0 && len(hosts.Image) == 0 {
		hosts = Hosts{API: apiHosts, Image: imageHosts}
	}
	timeout := s.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	rep := Report{Env: env}
	var mu sync.Mutex
	var tasks []probeTask
	record := func(t probeTask) {
		// 调度后各任务并发追加，汇总需要互斥。
		mu.Lock()
		defer mu.Unlock()
		tasks = append(tasks, t)
	}

	// #region 任务收集：解析端点与各主机的直连、代理路径
	// resolutions 由各主机的解析任务并发写入，全部任务结束后才汇总进报告。
	allHosts := append(append([]string(nil), hosts.API...), hosts.Image...)
	resolutions := make([]HostResolution, len(allHosts))
	{
		// 解析探测按主机清单逐台查询：API 与图片主机的解析结果可能不同，
		// 只查其中一台无法回答「各主机解析到了什么」。
		for i, h := range allHosts {
			record(probeTask{
				name: "解析查询（直连）", detail: h,
				run: func(ctx context.Context) error {
					ips, err := p.ProbeResolver(ctx, env.ResolverEndpoint, h, false)
					mu.Lock()
					// 解析结果只由直连查询产出；失败时 err 携带原因。
					rep.Resolver.Endpoint = env.ResolverEndpoint
					resolutions[i] = HostResolution{Host: h, IPs: ips, Err: err}
					mu.Unlock()
					return err
				},
			})
		}
		// 经代理的解析探测只对经 HTTP 查询的端点有意义：明文 DNS 走 UDP，
		// 系统解析走平台 API，两者都不受进程代理环境变量影响。
		// 查询哪台主机不影响「端点能否经代理查到」，取清单首台即可。
		if env.ProxyURL != nil && dns.EndpointUsesHTTP(env.ResolverEndpoint) && len(allHosts) > 0 {
			record(probeTask{
				name: "解析查询（经代理）", detail: env.ResolverEndpoint,
				run: func(ctx context.Context) error {
					_, err := p.ProbeResolver(ctx, env.ResolverEndpoint, allHosts[0], true)
					mu.Lock()
					rep.Resolver.ProxyOK = err == nil
					rep.Resolver.ProxyErr = err
					mu.Unlock()
					return err
				},
			})
		}
	}
	for _, h := range hosts.API {
		record(probeTask{
			name: "API 主机 ECH 直连", detail: h,
			run: func(ctx context.Context) error {
				err := p.ProbeECH(ctx, h)
				mu.Lock()
				if err == nil {
					rep.API.Direct = true
					recordDirectWay(&rep.API, "ECH")
				} else {
					rep.API.DirectErrs = append(rep.API.DirectErrs, Check{Name: "API 主机 ECH 直连", Detail: h, Err: err})
				}
				mu.Unlock()
				return err
			},
		})
		record(probeTask{
			name: "API 主机常规直连", detail: h,
			run: func(ctx context.Context) error {
				err := p.ProbeHTTPS(ctx, fmt.Sprintf("https://%s/", h), false)
				mu.Lock()
				if err == nil {
					rep.API.Direct = true
					recordDirectWay(&rep.API, "常规")
				} else {
					rep.API.DirectErrs = append(rep.API.DirectErrs, Check{Name: "API 主机常规直连", Detail: h, Err: err})
				}
				mu.Unlock()
				return err
			},
		})
		// API 主机在 ECH 之外还探测不发送 SNI 的路径：no-SNI 对 www.pixiv.net
		// 拨号解析到 pixiv.net 源站（见 client.NoSNIHostTarget），它接受不发送
		// SNI 的握手而 ECH 需落在 Cloudflare。
		record(probeTask{
			name: "API 主机无 SNI 直连", detail: h,
			run: func(ctx context.Context) error {
				err := p.ProbeNoSNI(ctx, h)
				mu.Lock()
				if err == nil {
					rep.API.Direct = true
					recordDirectWay(&rep.API, "无 SNI")
				} else {
					rep.API.DirectErrs = append(rep.API.DirectErrs, Check{Name: "API 主机无 SNI 直连", Detail: h, Err: err})
				}
				mu.Unlock()
				return err
			},
		})
	}
	for _, h := range hosts.Image {
		record(probeTask{
			name: "图片主机无 SNI 直连", detail: h,
			run: func(ctx context.Context) error {
				err := p.ProbeNoSNI(ctx, h)
				mu.Lock()
				if err == nil {
					rep.Image.Direct = true
					recordDirectWay(&rep.Image, "无 SNI")
				} else {
					rep.Image.DirectErrs = append(rep.Image.DirectErrs, Check{Name: "图片主机无 SNI 直连", Detail: h, Err: err})
				}
				mu.Unlock()
				return err
			},
		})
		record(probeTask{
			name: "图片主机常规直连", detail: h,
			run: func(ctx context.Context) error {
				err := p.ProbeHTTPS(ctx, fmt.Sprintf("https://%s/", h), false)
				mu.Lock()
				if err == nil {
					rep.Image.Direct = true
					recordDirectWay(&rep.Image, "常规")
				} else {
					rep.Image.DirectErrs = append(rep.Image.DirectErrs, Check{Name: "图片主机常规直连", Detail: h, Err: err})
				}
				mu.Unlock()
				return err
			},
		})
	}
	if env.ProxyURL != nil {
		for _, h := range hosts.API {
			record(probeTask{
				name: "API 主机经代理", detail: h,
				run: func(ctx context.Context) error {
					err := p.ProbeHTTPS(ctx, fmt.Sprintf("https://%s/", h), true)
					mu.Lock()
					rep.API.ViaProxy = rep.API.ViaProxy || err == nil
					if err != nil {
						rep.API.ViaProxyErr = err
					}
					mu.Unlock()
					return err
				},
			})
		}
		for _, h := range hosts.Image {
			record(probeTask{
				name: "图片主机经代理", detail: h,
				run: func(ctx context.Context) error {
					err := p.ProbeHTTPS(ctx, fmt.Sprintf("https://%s/", h), true)
					mu.Lock()
					rep.Image.ViaProxy = rep.Image.ViaProxy || err == nil
					if err != nil {
						rep.Image.ViaProxyErr = err
					}
					mu.Unlock()
					return err
				},
			})
		}
	}
	// #endregion

	// #region 并行调度：单项超时独立生效，顺序按任务收集顺序稳定呈现
	var wg sync.WaitGroup
	checks := make([]Check, len(tasks))
	for i, task := range tasks {
		wg.Add(1)
		go func(i int, task probeTask) {
			defer wg.Done()
			err := runWithTimeout(ctx, timeout, task.run)
			checks[i] = Check{Name: task.name, Detail: task.detail, Err: err}
		}(i, task)
	}
	wg.Wait()
	rep.Checks = checks
	// 逐台解析结果此时才完整：各解析任务已写入各自的槽位。
	rep.Resolver.Resolutions = resolutions
	// 端点是否可用按「任一主机解析成功」判定，与顺序无关：结论句说的
	// 「解析直连可用」指的是这个端点能用，单台主机查不到（例如该名字
	// 不存在）不代表端点坏了。
	for _, res := range resolutions {
		if res.Err == nil {
			rep.Resolver.DirectOK = true
		} else if rep.Resolver.DirectErr == nil {
			rep.Resolver.DirectErr = res.Err
		}
	}
	// #endregion

	return rep
}

// runWithTimeout 在独立超时下运行一次探测。
//
// 超时既取消探测的上下文（连接层据此中断等待），也不再等待探测函数返回
// ——它可能无视取消而阻塞，此时放弃其结果继续汇总，避免单个探测拖住整个套件。
// 探测对共享报告的写入在函数返回前完成与否不再影响结论：超时项记为失败。
func runWithTimeout(ctx context.Context, timeout time.Duration, run func(ctx context.Context) error) error {
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- run(probeCtx)
	}()
	select {
	case err := <-done:
		return err
	case <-probeCtx.Done():
		return fmt.Errorf("探测超时（%s 内未完成，视为不可用）", timeout)
	}
}
