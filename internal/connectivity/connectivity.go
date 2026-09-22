// Package connectivity 探测当前环境对本库各连接方式的可达性，并渲染为报告。
//
// 它是诊断工具（cmd/pixiv-doctor）的引擎，不提供稳定性承诺；
// 位于 internal，导出仅为测试与 cmd 装配的可见性。
//
// 报告回答四件事：
//
//  1. 环境多大程度上可以直连（ECH / 无 SNI / 常规）；
//  2. AutoTransport 是否可链接（与运行时行为一致：任一方式可达即能用）；
//  3. 配置了 HTTPS_PROXY 时，解析查询与数据传输（API / 图片）是否需要经过代理；
//  4. 各主机（API 与图片）分别解析到了什么地址。
package connectivity

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"

	"github.com/NateScarlet/pixiv/pkg/client/dns"
)

// Environment 收集探测所需的环境信息，由最外层装配。
//
// ProxyURL 为 nil 表示未配置代理；经代理的探测只在它非 nil 时执行。
type Environment struct {
	GoVersion string
	// HTTPSProxyRaw 是 HTTPS_PROXY 环境变量的原始值，仅在报告头部展示。
	HTTPSProxyRaw string
	// ProxyURL 是解析后的代理地址；nil 表示未配置代理。
	ProxyURL *url.URL
	// ResolverEndpoint 是 PIXIV_DNS_QUERY_URL 的生效值。
	ResolverEndpoint string
	// ResolverEndpointIsDefault 报告解析端点是否为内置默认值（用户未配置）。
	ResolverEndpointIsDefault bool
}

// Check 是一条探测记录，保留原始错误以供诊断。
type Check struct {
	// Name 是面向人的探测名称，如「API 主机 ECH 直连」。
	Name string
	// Detail 是探测对象（主机名或地址）。
	Detail string
	Err    error
}

// OK 报告该探测是否成功。
func (c Check) OK() bool { return c.Err == nil }

// Prober 提供单项探测能力，是网络实现的接缝；测试注入 fake，
// 真实实现见 live.go。
type Prober interface {
	// ProbeResolver 用指定端点查询主机，返回 A 记录的解析结果。
	// viaProxy 为 true 时强制经代理发出；仅「端点经 HTTP 查询」且环境配置了
	// 代理时被调用——明文 DNS 与系统解析没有可经代理的出网路径。
	ProbeResolver(ctx context.Context, endpoint, host string, viaProxy bool) ([]net.IP, error)
	// ProbeHTTPS 以常规方式请求该地址；viaProxy 为 true 时强制经代理发出。
	ProbeHTTPS(ctx context.Context, rawURL string, viaProxy bool) error
	// ProbeECH 对 Cloudflare 托管主机施加 ECH 直连。
	ProbeECH(ctx context.Context, host string) error
	// ProbeNoSNI 对图片主机以不发送 SNI 的方式直连。
	ProbeNoSNI(ctx context.Context, host string) error
}

// HostReport 汇总一类数据主机的探测结果。
type HostReport struct {
	// Direct 表示存在可用的直连方式，与 DirectWays 一致。
	Direct bool
	// DirectWays 列出可用的直连方式名（如「ECH」「常规」），空切片表示不可直连。
	DirectWays []string
	// DirectErrs 是各直连方式失败的原因，与结论一同展示。
	DirectErrs []Check
	// ViaProxy 报告经代理访问是否可达；未配置代理时恒为 false。
	ViaProxy bool
	// ViaProxyErr 是经代理访问失败的原因；未配置代理或访问成功时为 nil。
	ViaProxyErr error
}

// HostResolution 是一台主机经解析端点的查询结果。
//
// API 与图片主机各自解析到不同地址，逐台报告才能看出某类主机的解析
// 是否异常；只查一台会把「图片主机解析正常」误读成解析整体正常。
type HostResolution struct {
	// Host 是被查询的主机名。
	Host string
	// IPs 是解析到的地址；Err 非 nil 时无意义。
	IPs []net.IP
	// Err 是本次查询失败的原因。
	Err error
}

// ResolverReport 汇总解析端点的探测结果。
type ResolverReport struct {
	Endpoint string
	// DirectOK 报告不经代理的查询是否成功；任一主机解析成功即为 true，
	// 单台主机的失败不表示端点不可用（见 Resolutions）。
	DirectOK bool
	// DirectErr 是首次失败的查询原因，用于端点整体失败的归因。
	DirectErr error
	// Resolutions 是逐台主机的直连查询结果，按主机清单顺序排列。
	// 它是解析明细的来源；端点整体失败时各台主机同样失败。
	Resolutions []HostResolution
	// ProxyOK 报告经代理的查询是否成功；未配置代理、或解析方式不经 HTTP 时
	// 恒为 false——那些情形下没有可经代理的出网路径。
	ProxyOK  bool
	ProxyErr error
}

// NeedsProxy 报告解析查询是否需要经过代理：
// 仅在「配置了代理、直连查询失败、经代理查询成功」三者同时成立时为 true。
// 未配置代理时恒为 false——此时没有代理可走，「需要代理」无从谈起。
func (r ResolverReport) NeedsProxy() bool {
	return r.ProxyOK && !r.DirectOK
}

// 失败归因，用于把「端点不可达」细分为可操作的结论。
type failureKind int

const (
	// failureNone 表示没有失败。
	failureNone failureKind = iota
	// failureUnreachable 是连接层失败：端点连不上，与查询内容无关。
	failureUnreachable
	// failureTLS 是证书校验失败：连上了但对端证书不被信任。
	failureTLS
	// failureRejected 是端点返回了非成功状态码：端点可达，但拒绝了这次查询。
	failureRejected
	// failureUnparsable 是响应无法解析：端点可达且接受了查询，但响应不合预期。
	failureUnparsable
)

// classifyFailure 判定一次失败的性质。
//
// 三者的处置完全不同：不可达要查网络，证书失败要处理信任，
// 被拒绝要查端点的协议支持——归成同一句话会让排查方向全错。
func classifyFailure(err error) failureKind {
	if err == nil {
		return failureNone
	}
	// 端点返回了非成功状态码：可达但拒绝了查询。
	var statusErr *dns.StatusError
	if errors.As(err, &statusErr) {
		return failureRejected
	}
	// 响应无法按预期解读：可达且接受了查询，但内容不合预期。
	var parseErr *dns.ParseError
	if errors.As(err, &parseErr) {
		return failureUnparsable
	}
	// 证书校验失败在 Go 中是一个可判别的错误类型。
	var certErr *tls.CertificateVerificationError
	if errors.As(err, &certErr) {
		return failureTLS
	}
	// 连不上：拒绝连接、超时等。
	var netErr net.Error
	if errors.As(err, &netErr) {
		return failureUnreachable
	}
	// DNS 解析失败。
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return failureUnreachable
	}
	// 无法判别的失败按最保守的归因处理，不臆断原因。
	return failureUnreachable
}

// describeFailure 用一句话说明解析失败的性质，供结论句使用。
func describeFailure(err error) string {
	switch classifyFailure(err) {
	case failureTLS:
		return "解析端点证书不被信任"
	case failureRejected:
		return "解析端点拒绝了查询（端点可达，但不接受这种查询）"
	case failureUnparsable:
		return "解析端点的响应无法解析"
	case failureUnreachable:
		return "解析端点不可达"
	default:
		return "解析查询失败"
	}
}

// hintFailure 给出针对该失败性质的处置建议；无需额外说明时返回空串。
func hintFailure(err error) string {
	switch classifyFailure(err) {
	case failureRejected:
		return "端点可达但不接受查询，通常是它不实现当前编码方式：用 #type=json 或 #type=message 声明正确的方式"
	case failureTLS:
		return "自建端点需把其 CA 装入系统信任根"
	default:
		return ""
	}
}

// Report 是一次探测的完整结果。
type Report struct {
	Env      Environment
	Resolver ResolverReport
	API      HostReport
	Image    HostReport
	// Checks 是按执行顺序排列的全部探测记录，渲染为明细。
	Checks []Check
}

// AutoTransportUsable 报告默认传输在当前环境能否完成请求。
//
// 与运行时一致：首选直连方式失败时会回落到常规连接，因此每类主机
// 只要有任一可用路径（直连方式或代理）即可用。判断按主机类别聚合，
// 不区分同类别内的单台主机。
func (r Report) AutoTransportUsable() bool {
	apiOK := len(r.API.DirectWays) > 0 || r.API.ViaProxy
	imageOK := len(r.Image.DirectWays) > 0 || r.Image.ViaProxy
	return apiOK && imageOK
}

// 结论等级，从优到劣；数值顺序仅用于渲染分组，不参与比较语义。
type Level int

const (
	// LevelDirect 数据传输可完全直连。
	LevelDirect Level = iota
	// LevelPartial 部分数据（API 或图片之一）可直连。
	LevelPartial
	// LevelProxyRequired 数据传输需要经过代理。
	LevelProxyRequired
	// LevelUnavailable 没有任何可用路径。
	LevelUnavailable
)

// Verdict 是面向人的结论。
type Verdict struct {
	Level Level
	Text  string
}

// Verdict 依据探测结果组合得出结论，不解释原因；各方式失败原因在明细中展示。
func (r Report) Verdict() Verdict {
	apiDirect := len(r.API.DirectWays) > 0
	imageDirect := len(r.Image.DirectWays) > 0

	var v Verdict
	switch {
	case apiDirect && imageDirect:
		v.Level = LevelDirect
		v.Text = fmt.Sprintf("数据传输可完全直连（API：%s；图片：%s）",
			strings.Join(r.API.DirectWays, "、"), strings.Join(r.Image.DirectWays, "、"))
	case apiDirect || imageDirect:
		// 仅部分类别可直连也是可用状态：API 能用而图片不能是「部分可用」，
		// 不能因某类失败就把整体说成不可用。
		v.Level = LevelPartial
		v.Text = fmt.Sprintf("部分可用：API %s，图片 %s", hostState(r.API), hostState(r.Image))
	case r.API.ViaProxy || r.Image.ViaProxy:
		v.Level = LevelProxyRequired
		v.Text = "没有可用的直连方式，数据传输需要经过代理"
	default:
		v.Level = LevelUnavailable
		if r.Env.ProxyURL == nil {
			v.Text = "没有可用路径：直连不可达，也未配置代理"
		} else {
			v.Text = "没有可用路径：直连与代理均不可达，请检查代理地址与网络"
		}
	}
	v.Text += "；" + r.resolverClause()
	if clause := r.dataProxyClause(); clause != "" {
		v.Text += "；" + clause
	}
	return v
}

// hostState 用一句话描述一类主机的可达状态。
func hostState(h HostReport) string {
	if len(h.DirectWays) > 0 {
		return "可直连（" + strings.Join(h.DirectWays, "、") + "）"
	}
	if h.ViaProxy {
		return "需要经过代理"
	}
	return "没有可达路径"
}

// resolverClause 用一句话描述解析查询与代理的关系。
//
// 失败时按真实原因归因：「端点不可达」「证书不被信任」「端点拒绝了查询」
// 是三件不同的事，混为一谈会把排查方向带偏。
func (r Report) resolverClause() string {
	if r.Env.ProxyURL == nil {
		if r.Resolver.DirectOK {
			return "解析直连可用"
		}
		return resolverFailureClause(r.Resolver.DirectErr)
	}
	switch {
	case r.Resolver.NeedsProxy():
		// 经 HTTP 的解析查询走 http.DefaultClient，遵循 HTTPS_PROXY，当前配置已满足该需求。
		return "解析需要经过代理（运行时已自动满足：DoH 查询遵循 HTTPS_PROXY）"
	case r.Resolver.DirectOK:
		return "解析无需经过代理"
	case r.Resolver.ProxyOK:
		return "解析直连不可用，经代理可用"
	default:
		// 两条路径都失败：直连的失败原因更能说明端点本身的问题
		// （代理路径的失败可能只是代理不可用）。
		return resolverFailureClause(r.Resolver.DirectErr)
	}
}

// resolverFailureClause 依据解析失败的原因给出结论与处置建议。
func resolverFailureClause(err error) string {
	text := describeFailure(err)
	if hint := hintFailure(err); hint != "" {
		text += "——" + hint
	}
	return text
}

// dataProxyClause 用一句话描述数据传输对代理的依赖；
// dataProxyClause 用一句话描述数据传输对代理的依赖。
//
// 仅在全部数据都可直连时补充说明「无需代理」；某类数据需要代理的情形
// 已由结论主句（hostState）表达，不再重复。
func (r Report) dataProxyClause() string {
	if r.Env.ProxyURL == nil {
		return ""
	}
	if len(r.API.DirectWays) > 0 && len(r.Image.DirectWays) > 0 {
		return "数据传输无需经过代理"
	}
	return ""
}

// Render 把报告渲染为面向人的文本。
func Render(w io.Writer, r Report) error {
	if _, err := fmt.Fprintf(w, "pixiv 连通性检测\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "环境: %s\n", r.Env.GoVersion); err != nil {
		return err
	}
	if r.Env.ProxyURL == nil {
		if _, err := fmt.Fprintf(w, "代理: 未配置\n"); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintf(w, "代理: %s（HTTPS_PROXY）\n", r.Env.HTTPSProxyRaw); err != nil {
			return err
		}
	}
	endpoint := r.Env.ResolverEndpoint
	if r.Env.ResolverEndpointIsDefault {
		endpoint += "（默认，可用 PIXIV_DNS_QUERY_URL 更换）"
	}
	if _, err := fmt.Fprintf(w, "解析端点: %s\n", endpoint); err != nil {
		return err
	}

	v := r.Verdict()
	if _, err := fmt.Fprintf(w, "\n结论: %s\n", v.Text); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "AutoTransport: %s\n", autoTransportState(r)); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(w, "\n明细:\n"); err != nil {
		return err
	}
	for _, c := range r.Checks {
		if err := renderCheck(w, c); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintf(w, "\n解析结果:\n"); err != nil {
		return err
	}
	for _, res := range r.Resolver.Resolutions {
		if err := renderResolution(w, res); err != nil {
			return err
		}
	}
	return nil
}

// renderResolution 渲染一台主机的解析结果。
//
// 解析失败的每台主机各自成行并带失败原因：某类主机被解析到错误地址
// （例如被投毒或走了错误的解析端点）时，逐台对照才看得出来。
func renderResolution(w io.Writer, res HostResolution) error {
	state := ""
	switch {
	case res.Err != nil:
		state = fmt.Sprintf("查询失败: %s", res.Err.Error())
	case len(res.IPs) == 0:
		state = "未解析到地址"
	default:
		ips := make([]string, len(res.IPs))
		for i, ip := range res.IPs {
			ips[i] = ip.String()
		}
		state = strings.Join(ips, ", ")
	}
	_, err := fmt.Fprintf(w, "  %s: %s\n", res.Host, state)
	return err
}

// autoTransportState 用一句话描述默认传输的可用性。
//
// 与运行时一致的口径是按主机类别聚合：所有类别都有可达路径才是可用；
// 仅部分类别可用时如实报告部分可用——API 能直连而图片不能，
// 对调用者是「部分可用」，不是「不可用」。
func autoTransportState(r Report) string {
	apiOK := len(r.API.DirectWays) > 0 || r.API.ViaProxy
	imageOK := len(r.Image.DirectWays) > 0 || r.Image.ViaProxy
	switch {
	case apiOK && imageOK:
		return "可用（存在可达路径，首选方式失败会自动回落）"
	case apiOK || imageOK:
		return "部分可用（仅部分主机类别存在可达路径）"
	default:
		return "不可用（没有任何可达路径）"
	}
}

// renderCheck 渲染一条探测记录，错误原样呈现——报告是给用户自己看的诊断输出。
func renderCheck(w io.Writer, c Check) error {
	mark, state := "[ok]  ", "成功"
	if !c.OK() {
		mark, state = "[失败]", fmt.Sprintf("失败: %s", c.Err.Error())
	}
	detail := c.Detail
	if detail != "" {
		detail = " " + detail
	}
	_, err := fmt.Fprintf(w, "%s %s%s → %s\n", mark, c.Name, detail, state)
	return err
}
