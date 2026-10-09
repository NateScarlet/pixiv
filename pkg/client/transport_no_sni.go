package client

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
)

// noSNIServerName 用于抑制 SNI 扩展。
//
// crypto/tls 对 IP 字面量不生成 SNI 扩展（RFC 6066），因此把 ServerName 设为
// 任意 IP 字面量即可不发送 SNI，同时保留与代理共存的能力——DialTLSContext
// 只对 non-proxied 请求生效，经代理时会被静默忽略，故不使用它。
const noSNIServerName = "0.0.0.0"

// NewNoSNITransport 返回一个不发送 SNI 的传输原语：在 base 之上叠加
// 「握手时不携带 SNI」这一种连接能力。
//
// 它不含主机判断，也不含环境判断：单独使用它的人需要自己决定何时用它。
// 若需要「按主机自动选用」，用 [NewRoutedTransport]。
//
// 不发送 SNI 后标准库无法自动按主机名校验证书，因此本原语改为自行校验证书链。
// 由于原语不知道目标主机名，它只校验证书链；主机名校验需要知道请求主机，
// 由 [NewRoutedTransport] 在收到响应后补齐。
// 需要更严格的校验时，调用者可在 base 上自行设置 VerifyPeerCertificate。
//
// 服务端以 403 拒绝时本原语报错而不是把响应交给调用者：不发送 SNI 的握手不含
// 主机名，服务端只能落到默认 server block，因此这类 403 是「这条腿对该主机
// 不可用」的证据。调用者拿到的错误经 errors.As 可取回 [*ErrAPIRejected]（其中的
// 响应体已关闭，仅供读取状态）。业务状态码不构成拒绝这条腿的证据，原样返回。
//
// 返回的传输是承载上述能力的不透明 [http.RoundTripper]：它克隆自 base，因此可与
// 其他原语嵌套组合，且不改变调用者的 base。调用者只依赖 http.RoundTripper，
// 库不暴露底层传输。
func NewNoSNITransport(base *http.Transport) *noSNITransport {
	if base == nil {
		// 未指定时使用进程默认传输。本原语不承担解析：需要经注入的解析器或
		// 不同的解析目标（别名）时，由调用者用 NewHostAliasTransport 组合 base。
		base = defaultBaseTransport()
	}
	var t = base.Clone()
	var cfg *tls.Config
	if t.TLSClientConfig == nil {
		cfg = new(tls.Config)
	} else {
		cfg = t.TLSClientConfig.Clone()
	}
	cfg.ServerName = noSNIServerName
	if !cfg.InsecureSkipVerify {
		cfg.InsecureSkipVerify = true
		// 调用者原有的校验回调仍然生效，叠加而非替换，以便组合时不丢失调用者的要求。
		cfg.VerifyPeerCertificate = chainVerifyPeerCertificate(
			verifyPeerCertificate(cfg.RootCAs),
			cfg.VerifyPeerCertificate,
		)
	}
	t.TLSClientConfig = cfg
	// 拨号与解析（含目标主机别名）全部委托给 base，本原语只叠加「不发送 SNI」。
	return &noSNITransport{base: t}
}

// noSNITransport 承载「不发送 SNI」与「被服务端拒绝即报错」两种能力。
//
// 判定放在这里而不是自动选择传输的候选循环里：它最清楚自己是什么腿，也最清楚
// 自己何时被服务端拒绝。调用者只把它当作 [http.RoundTripper] 使用。
type noSNITransport struct {
	base *http.Transport
}

// RoundTrip implements http.RoundTripper
func (t *noSNITransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil || resp.StatusCode != http.StatusForbidden {
		return resp, err
	}
	// 响应不可用，关闭它以免连接与正文泄漏给一个没人会消费的应答；
	// 关闭失败也是调用者可见的诊断信息。
	return nil, errors.Join(&ErrAPIRejected{Response: resp}, resp.Body.Close())
}

// CloseIdleConnections 关闭空闲连接，使 [http.Client.CloseIdleConnections] 对本
// 传输仍然有效——否则调用者只能经 http.RoundTripper 使用本原语，无从释放连接。
func (t *noSNITransport) CloseIdleConnections() {
	t.base.CloseIdleConnections()
}

// chainVerifyPeerCertificate 依次执行两个校验回调，任一失败即返回该错误。
func chainVerifyPeerCertificate(
	first, second func([][]byte, [][]*x509.Certificate) error,
) func([][]byte, [][]*x509.Certificate) error {
	if second == nil {
		return first
	}
	return func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
		if err := first(rawCerts, verifiedChains); err != nil {
			return err
		}
		return second(rawCerts, verifiedChains)
	}
}

// verifyPeerCertificate 校验证书链。
//
// 这是 InsecureSkipVerify 的替代校验：不发送 SNI 时标准库无法自动完成这一步。
// 主机名不在这里校验：握手阶段拿不到目标主机名（拨号目标只到拨号为止），
// 需要主机名的校验由 [verifyResponseHostname] 在收到响应后补齐。
func verifyPeerCertificate(roots *x509.CertPool) func([][]byte, [][]*x509.Certificate) error {
	return func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
		var certs, err = parsePeerCertificates(rawCerts)
		if err != nil {
			return err
		}
		var opts = x509.VerifyOptions{
			Roots:         roots,
			Intermediates: x509.NewCertPool(),
		}
		for _, cert := range certs[1:] {
			opts.Intermediates.AddCert(cert)
		}
		if _, err := certs[0].Verify(opts); err != nil {
			return fmt.Errorf("pixiv: client: 服务器证书不可信: %w", err)
		}
		return nil
	}
}

// verifyResponseHostname 用响应中实际协商出的证书校验主机名。
//
// 不发送 SNI 时标准库无法自动完成这一步，而握手阶段又拿不到目标主机名，
// 因此由知道请求主机的路由层在收到响应后校验：校验不通过说明连接的对端
// 并非该主机，响应内容不可信，应报错而不是把它交给调用者。
func verifyResponseHostname(resp *http.Response, host string) error {
	if resp.TLS == nil {
		// 请求未经 TLS（例如测试用的明文端点），没有证书可校验。
		return nil
	}
	var certs = resp.TLS.PeerCertificates
	if len(certs) == 0 {
		return errors.New("pixiv: client: 服务器未提供证书")
	}
	if err := certs[0].VerifyHostname(host); err != nil {
		return fmt.Errorf("pixiv: client: 服务器证书与主机 %s 不匹配: %w", host, err)
	}
	return nil
}

// parsePeerCertificates 把握手得到的原始证书解析为证书链。
func parsePeerCertificates(rawCerts [][]byte) ([]*x509.Certificate, error) {
	if len(rawCerts) == 0 {
		return nil, errors.New("pixiv: client: 服务器未提供证书")
	}
	var certs = make([]*x509.Certificate, len(rawCerts))
	for i, raw := range rawCerts {
		var cert, err = x509.ParseCertificate(raw)
		if err != nil {
			return nil, fmt.Errorf("pixiv: client: 无法解析服务器证书: %w", err)
		}
		certs[i] = cert
	}
	return certs, nil
}
