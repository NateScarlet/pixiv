package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
)

// ECHDefaultPublicName 是施加 ECH 时的外层 SNI。
//
// 该名字写在 ECHConfig 的 public_name 字段内，是 Cloudflare 持有证书、
// 用于完成外层握手的公共名。实测改写该字段为其他名字会握手失败，
// 因此默认值不应被改动；仅当目标使用其他 ECH 提供方时才需要通过选项指定。
const ECHDefaultPublicName = "cloudflare-ech.com"

// ECHOption 描述构造 ECH 传输原语的一项设置。
//
// 与 [Option] 一样是带未导出方法的接口，从而封闭实现集合，
// 调用者无法写出本包未预期的选项。
type ECHOption interface {
	applyToECH(*echConfig)
}

// echConfig 收集 ECH 原语的显式设置。
type echConfig struct {
	configList []byte
	publicName string
}

type echOptionFunc func(*echConfig)

func (f echOptionFunc) applyToECH(c *echConfig) { f(c) }

// WithECHConfigList 用调用者提供的 ECHConfigList 施加 ECH。
//
// 未提供时原语会自行取得配置。配置会轮换，自行提供一份静态配置意味着
// 配置过期后连接会失败，直到调用者更新它；原语仍会在服务端拒绝时
// 用下发的 retry_configs 自愈。
func WithECHConfigList(list []byte) ECHOption {
	return echOptionFunc(func(c *echConfig) { c.configList = list })
}

// WithECHPublicName 指定 ECH 的外层名（public_name）。
//
// 默认值为 [ECHDefaultPublicName]，即 Cloudflare 的公共名，适用于访问托管在
// Cloudflare 的主机。**该值不是可随意改动的参数**：它必须是 ECH 提供方持有
// 证书的公共名，实测改写为等长的其它名字会导致握手失败，使用其它组织
// （如 defo.ie、tls-ech.dev）的配置同样失败。
//
// 因此只有目标确实使用其它 ECH 提供方时才需要设置它。
func WithECHPublicName(name string) ECHOption {
	return echOptionFunc(func(c *echConfig) { c.publicName = name })
}

// NewECHTransport 返回一个施加 ECH 的传输：在 base 之上叠加
// 「用加密的 ClientHello 连接」这一种连接能力。
//
// ECH 把 ClientHello 拆成内外两层：外层使用公共的 cloudflare-ech.com 作为 SNI，
// 内层才是真实主机名且被加密，中间设备只能看到外层名字。因此本传输适用于
// **按 SNI 封锁但目标主机托管在 Cloudflare** 的场景。
//
// # 适用范围
//
// 只适用于托管在 Cloudflare 的主机。对不在 Cloudflare 之后的主机（例如 pixiv
// 自有源站 i.pximg.net），其证书与 ECH 的外层名不匹配，ECH 不适用。
//
// # 配置来源
//
// 两种方式：调用者用 [WithECHConfigList] 提供，或由本传输自行取得。自行取得
// 经 TLS 握手自举完成，**不需要 DNS**，也不需要外部文件：先发送一份结构合法但
// 服务端无法解密的配置，服务端拒绝时会在 HelloRetryRequest 中下发真正的配置。
//
// # 轮换自愈
//
// 配置会轮换，同一时刻新旧配置可能在不同边缘节点并存。服务端无法解密时会下发
// retry_configs，本传输用它重试并记住新配置，因此不需要重启或外部定时任务。
// 服务端明确拒绝且未下发配置时返回错误，不静默退回明文握手。
//
// # 与代理的关系
//
// ECH 主机的数据连接**不走代理**，即使 base 配置了代理（包括 HTTPS_PROXY
// 环境变量）也直连。ECH 的意义就是直连时绕开按 SNI 的封锁；经代理时封锁本就
// 被代理绕过，ECH 不再有意义，而且会让「ECH 是否生效」失去可观测性。
// 绕开代理只作用于本传输发出的请求：base 自身的代理设置不被修改，
// 调用者的其他传输照常使用代理。
//
// 需要 TLS 1.3。本传输不使用 DialTLSContext：标准库文档明确后者只对 non-proxied
// 请求生效，存在代理时被静默忽略，能力不生效且无任何提示。TLSClientConfig 承载
// ECH，直连所需的拨号能力由 base 提供。
//
// 返回的传输不改变调用者的 base，因此可与其他原语嵌套组合。
//
// 返回类型是 [http.RoundTripper] 而非 *http.Transport：自行取得配置与配置轮换
// 都发生在运行时，而 EncryptedClientHelloConfigList 是每 *http.Transport 一份、
// 无法按请求切换的字段，必须换用一份带新配置的传输才能表达，故以包装型实现。
func NewECHTransport(base *http.Transport, opts ...ECHOption) http.RoundTripper {
	return newECHTransportState(base, opts...)
}

// echTransport 是 ECH 传输的实现：它持有一份可更换配置的底层 Transport。
//
// EncryptedClientHelloConfigList 是每 Transport 一份、无法按请求切换的字段，
// 因此「运行时取得配置」不能靠修改字段实现，而必须换用一份带新配置的 Transport。
// 本类型即承载该替换：请求经它转发到当前配置对应的 Transport。
type echTransport struct {
	// base 提供拨号、代理与其余 TLS 设置；克隆自调用者的传输。
	base *http.Transport

	publicName string

	mu         sync.Mutex
	configList []byte
	active     *http.Transport
	// fetchMu 串行化自举，使并发的首批请求共用同一次取得。
	fetchMu sync.Mutex
}

// newECHTransportState 依据选项构造 ECH 传输内部状态。
func newECHTransportState(base *http.Transport, opts ...ECHOption) *echTransport {
	var cfg echConfig
	for _, o := range opts {
		if o == nil {
			continue
		}
		o.applyToECH(&cfg)
	}
	if cfg.publicName == "" {
		cfg.publicName = ECHDefaultPublicName
	}
	if base == nil {
		base = defaultBaseTransport()
	}
	t := &echTransport{
		base:       base.Clone(),
		publicName: cfg.publicName,
		configList: cfg.configList,
	}
	// 无论是否已有配置都先建好活跃传输：未提供配置时它不带 ECH 字段，
	// 取到配置后再换用带配置的一份。
	t.active = t.transportWith(cfg.configList)
	return t
}

// transportWith 返回一份带指定配置的传输；传 nil 表示不施加 ECH。
//
// 这个方法就是「换配置」的实现：每份传输带一份不可变的配置。
func (t *echTransport) transportWith(configList []byte) *http.Transport {
	var out = t.base.Clone()
	var tlsCfg *tls.Config
	if out.TLSClientConfig == nil {
		tlsCfg = new(tls.Config)
	} else {
		tlsCfg = out.TLSClientConfig.Clone()
	}
	// ECH 只在 TLS 1.3 下可用；显式抬高下限，使版本不满足时给出明确原因，
	// 而不是让 ECH 静默不生效。
	tlsCfg.MinVersion = tls.VersionTLS13
	if len(configList) > 0 {
		tlsCfg.EncryptedClientHelloConfigList = configList
	}
	// ECH 被拒时标准库会跳过 VerifyConnection / VerifyPeerCertificate，只调用本回调；
	// 若不提供，证书校验会退化为用外层名进行，调用者拿到的是证书错误而不是
	// *tls.ECHRejectionError，也就拿不到 retry_configs、无法自愈。
	tlsCfg.EncryptedClientHelloRejectionVerify = func(cs tls.ConnectionState) error {
		return verifyECHOuterCert(cs, t.publicName, tlsCfg.RootCAs)
	}
	out.TLSClientConfig = tlsCfg
	// ECH 的用途是直连时绕开按 SNI 的封锁，因此数据连接不走代理：
	// 经代理时封锁本就被代理绕过，ECH 不再有意义，且会让「ECH 是否生效」
	// 失去可观测性。这里清空代理设置，只保留底层传输的拨号能力。
	//
	// 调用者的拨号函数仍需保留：它承载注入的解析器（见 [resolverDialContext]），
	// 直连时正是靠它避开被污染的系统解析。
	out.Proxy = nil
	return out
}

// current 返回当前配置对应的传输。
func (t *echTransport) current() *http.Transport {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.active
}

// setConfig 换用一份带新配置的传输。
//
// 空配置被忽略：它表示服务端明确拒绝且无可重试配置，不是一份可用配置，
// 换上去只会让后续请求同样失败。
func (t *echTransport) setConfig(configList []byte) {
	if len(configList) == 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.configList = configList
	t.active = t.transportWith(configList)
}

// getConfig 返回当前配置；尚未取得时返回 nil。
func (t *echTransport) getConfig() []byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.configList
}

// RoundTrip implements http.RoundTripper
//
// 首次使用时若尚无配置则先自行取得；之后请求交给当前配置对应的传输。
func (t *echTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// 尚无配置：先取得再发出请求。取得途径是 TLS 握手自举，不需要 DNS。
	// 并发的首批请求共用同一次取得，避免各自发起一次握手。
	if err := t.ensureConfig(req.Context(), req.URL.Hostname(), req.URL.Port()); err != nil {
		return nil, err
	}

	resp, err := t.current().RoundTrip(req)
	if err == nil {
		return resp, nil
	}
	// 服务端下发了新配置：换用它重试一次，实现配置轮换时的自愈。
	var rej *tls.ECHRejectionError
	if !errors.As(err, &rej) {
		return resp, err
	}
	if len(rej.RetryConfigList) == 0 {
		// 服务端明确拒绝且无可重试配置：这是有意义的信号，不无限重试。
		// 此时首次尝试的响应不可用（握手即失败），故不返回它。
		return nil, fmt.Errorf("pixiv: client: ECH 被服务端拒绝且未提供可用配置（主机 %s）: %w",
			req.URL.Hostname(), ErrECHRejected)
	}
	// 首次尝试若已消费请求体，则无法原样重发（与标准库处理重定向的做法一致）；
	// 此时返回首次尝试的错误而非其响应：非 nil 响应与非 nil 错误并返不符合
	// http.RoundTripper 的约定，会让调用者拿到一个不可用的响应。
	if req.Body != nil && req.GetBody == nil {
		return nil, err
	}
	retryReq := req
	if req.Body != nil {
		body, bodyErr := req.GetBody()
		if bodyErr != nil {
			return nil, errors.Join(err, bodyErr)
		}
		retryReq = req.Clone(req.Context())
		retryReq.Body = body
	}
	t.setConfig(rej.RetryConfigList)
	return t.current().RoundTrip(retryReq)
}

// ensureConfig 保证配置已取得，必要时发起一次自举。
//
// 并发的首批请求共用同一次自举：取得过程只发生一次，失败时各调用者都拿到该错误。
func (t *echTransport) ensureConfig(ctx context.Context, host, port string) error {
	if len(t.getConfig()) > 0 {
		return nil
	}
	t.fetchMu.Lock()
	defer t.fetchMu.Unlock()
	// 等锁期间可能已有其他请求取得配置，此时无需再取。
	if len(t.getConfig()) > 0 {
		return nil
	}
	return t.fetchConfig(ctx, host, port)
}

// verifyECHOuterCert 校验 ECH 被拒时的外层证书。
//
// 被拒绝意味着内层握手未建立，此时对端出示的是 ECH 提供方（public_name）的证书，
// 因此按外层名校验。返回空错误使标准库继续把服务端下发的 retry_configs 通过
// *tls.ECHRejectionError 暴露出来；返回错误则中止握手。
//
// 注意 ConnectionState.PeerCertificates 在此回调中**必定为空**：标准库先在
// echRejected 分支调用本回调，之后才把解析好的证书写入 c.peerCertificates。
// 因此这里只能据此判断「对端未出示证书」，无法复核外层证书内容；
// 真正的外层校验只能依赖本回调被调用前的握手结果。
func verifyECHOuterCert(cs tls.ConnectionState, publicName string, roots *x509.CertPool) error {
	if len(cs.PeerCertificates) == 0 {
		// 标准库尚未填充该字段，见上文；此时不做校验，交由后续流程处理。
		return nil
	}
	opts := x509.VerifyOptions{
		Roots:         roots,
		DNSName:       publicName,
		Intermediates: x509.NewCertPool(),
	}
	for _, cert := range cs.PeerCertificates[1:] {
		opts.Intermediates.AddCert(cert)
	}
	if _, err := cs.PeerCertificates[0].Verify(opts); err != nil {
		return fmt.Errorf("pixiv: client: ECH 外层证书不可信: %w", err)
	}
	return nil
}

// bootstrapConn 建立一条用于自举的连接。
//
// 它直接复用底层传输的拨号与代理处理：把 probeTLS 挂在一份克隆的传输上，
// 用 net/http 自己的代理与 CONNECT 实现去连接，因此不重复实现代理协商与认证。
//
// bootstrapHandshake 用给定配置发起一次自举握手，并返回服务端下发的 retry_configs。
//
// 自举与数据连接走同一条直连路径，不使用代理：它取回的配置正是给直连用的，
// 经代获取的配置与直连时的实际行为不对应。
//
// 目标主机名经请求上下文中注入的解析器解析（见 [resolverDialContext]）：
// 系统解析可能返回被污染的地址，自举不应假设它可用。
func (t *echTransport) bootstrapHandshake(
	ctx context.Context, host, port string, probeTLS *tls.Config,
) ([]byte, error) {
	addr := net.JoinHostPort(host, port)
	rawConn, err := resolverDialContext(t.base.DialContext, host)(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("pixiv: client: ECH 自举连接 %s 失败: %w", host, err)
	}
	conn := tls.Client(rawConn, probeTLS)
	defer conn.Close()
	return retryConfigsFromHandshake(conn.HandshakeContext(ctx))
}

// retryConfigsFromHandshake 从握手结果中取出服务端下发的 retry_configs。
func retryConfigsFromHandshake(err error) ([]byte, error) {
	if err == nil {
		// 握手成功说明服务端接受了这份配置，而它对应的私钥并不在服务端手里，
		// 因此不该发生；按失败处理，避免把异常掩盖成「已取到配置」。
		return nil, errors.New("pixiv: client: ECH 自举握手意外成功，未能取得配置")
	}
	var rej *tls.ECHRejectionError
	if !errors.As(err, &rej) {
		return nil, fmt.Errorf("pixiv: client: ECH 自举握手失败: %w", err)
	}
	return rej.RetryConfigList, nil
}

// fetchConfig 用 TLS 握手自举取得 ECH 配置。
//
// 自举不需要 DNS：host 来自请求目标，不额外解析别的名字；拨号与代理都交给
// 底层传输，因此经代理时同样可用。它也不需要外部文件：发送一份服务端无法解密的
// 配置，从服务端下发的 retry_configs 中取回真正的配置。
func (t *echTransport) fetchConfig(ctx context.Context, host, port string) error {
	if host == "" {
		return errors.New("pixiv: client: 无法确定 ECH 自举的目标主机")
	}
	if port == "" {
		port = "443"
	}
	probe, err := bootstrapECHConfigList(t.publicName)
	if err != nil {
		return err
	}

	probeTLS := &tls.Config{
		MinVersion:                     tls.VersionTLS13,
		ServerName:                     host,
		EncryptedClientHelloConfigList: probe,
		// 自举只关心服务端下发的配置，外层证书是否可信不影响能否取到它。
		EncryptedClientHelloRejectionVerify: func(tls.ConnectionState) error { return nil },
	}
	if baseTLS := t.base.TLSClientConfig; baseTLS != nil {
		probeTLS.RootCAs = baseTLS.RootCAs
		probeTLS.NextProtos = baseTLS.NextProtos
	}

	retry, err := t.bootstrapHandshake(ctx, host, port, probeTLS)
	if err != nil {
		return err
	}
	if len(retry) == 0 {
		return fmt.Errorf("pixiv: client: 主机 %s 不接受 ECH，无法取得配置: %w", host, ErrECHRejected)
	}
	t.setConfig(retry)
	return nil
}
