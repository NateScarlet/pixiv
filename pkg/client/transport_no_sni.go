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

// noSNIHostnames 列出需要不发送 SNI 才能取回内容的主机。
//
// 这是本库掌握的 pixiv 主机布局知识，不对外暴露为选项：调用者无法观测
// pixiv 侧的变化，做成配置等于把一个无法完成的任务转嫁出去。
var noSNIHostnames = map[string]struct{}{
	// Pixiv 自有源站，接受不携带 SNI 的握手，配合 Referer 即可取图。
	"i.pximg.net": {},
}

// NewNoSNITransport 返回一个不发送 SNI 的传输原语：在 base 之上叠加
// 「握手时不携带 SNI」这一种连接能力。
//
// 它不含主机判断，也不含环境判断：单独使用它的人需要自己决定何时用它。
// 若需要「按主机自动选用」，用 [NewRoutedTransport]。
//
// 不发送 SNI 后标准库无法自动按主机名校验证书，因此本原语改为自行校验证书链。
// 由于原语不知道目标主机名，单独使用时只校验证书链；由 [NewRoutedTransport]
// 使用时还会校验证书主机名。需要更严格的校验时，调用者可在返回的传输上
// 自行设置 VerifyPeerCertificate。
//
// 返回的传输克隆自 base，因此可与其他原语嵌套组合，且不改变调用者的 base。
func NewNoSNITransport(base *http.Transport) *http.Transport {
	return newNoSNITransport(base, "")
}

func newNoSNITransport(base *http.Transport, host string) *http.Transport {
	if base == nil {
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
			verifyPeerCertificate(host, cfg.RootCAs),
			cfg.VerifyPeerCertificate,
		)
	}
	t.TLSClientConfig = cfg
	t.DialContext = resolverDialContext(t.DialContext, host)
	return t
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

// verifyPeerCertificate 校验服务器证书链，并在已知主机名时校验主机名。
//
// 这是 InsecureSkipVerify 的替代校验：不发送 SNI 时标准库无法自动完成这一步。
func verifyPeerCertificate(host string, roots *x509.CertPool) func([][]byte, [][]*x509.Certificate) error {
	return func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
		if len(rawCerts) == 0 {
			return errors.New("pixiv: client: 服务器未提供证书")
		}
		var certs = make([]*x509.Certificate, len(rawCerts))
		for i, raw := range rawCerts {
			var cert, err = x509.ParseCertificate(raw)
			if err != nil {
				return fmt.Errorf("pixiv: client: 无法解析服务器证书: %w", err)
			}
			certs[i] = cert
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
		if host == "" {
			return nil
		}
		if err := certs[0].VerifyHostname(host); err != nil {
			return fmt.Errorf("pixiv: client: 服务器证书与主机 %s 不匹配: %w", host, err)
		}
		return nil
	}
}
