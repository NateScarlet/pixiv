package connectivity

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/NateScarlet/pixiv/pkg/client/dns"
	"github.com/stretchr/testify/assert"
)

// TestClassifyFailure 断言各类失败被归到不同的性质。
//
// 归因决定排查方向：证书问题要处理信任，被拒绝要查端点协议支持，
// 连不上才要查网络。三者混为一谈会让用户照着错误的线索排查。
func TestClassifyFailure(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
		want failureKind
	}{
		{"无失败", nil, failureNone},
		{
			"证书不被信任",
			fmt.Errorf("包装: %w", &tls.CertificateVerificationError{
				Err: errors.New("x509: certificate signed by unknown authority"),
			}),
			failureTLS,
		},
		{
			"端点返回非成功状态码",
			fmt.Errorf("包装: %w", &dns.StatusError{StatusCode: 400, Status: "Bad Request"}),
			failureRejected,
		},
		{
			"响应无法解析",
			fmt.Errorf("包装: %w", &dns.ParseError{}),
			failureUnparsable,
		},
		{"连接超时", &net.OpError{Op: "dial", Err: errors.New("i/o timeout")}, failureUnreachable},
		{"DNS 失败", &net.DNSError{Err: "no such host", Name: "x"}, failureUnreachable},
		{"无法判别的失败按不可达处理", errors.New("未知错误"), failureUnreachable},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, classifyFailure(tt.err))
		})
	}
}

// TestStatusErrorTakesPrecedenceOverNetwork 断言「端点拒绝查询」不会被
// 误判为网络问题：400 这类应答证明端点可达，那是协议不支持而非连不上。
func TestStatusErrorTakesPrecedenceOverNetwork(t *testing.T) {
	// 解析器把状态码包装在自身上下文里，判定必须能穿透包装看到状态码。
	err := fmt.Errorf("dohResolver{'https://doh.example/dns-query'}.Resolve('i.pximg.net'): %w",
		&dns.StatusError{StatusCode: 400, Status: "Bad Request"})
	assert.Equal(t, failureRejected, classifyFailure(err))
	assert.Contains(t, describeFailure(err), "拒绝了查询")
}

// TestDohClauseReportsRejection 断言结论句说明端点拒绝了查询并给出
// 可操作的处置建议，而不是笼统的「端点不可达」。
//
// 场景还原：dnscrypt-proxy 的本地 DoH 只实现 RFC 8484 二进制接口，
// 用户若把端点写成 #type=json 就会得到 400。用户需要知道要改声明的编码方式。
func TestDohClauseReportsRejection(t *testing.T) {
	r := Report{
		DoH: DoHReport{
			Endpoint:  "https://doh.home.arpa:3443/dns-query#type=json",
			DirectErr: &dns.StatusError{StatusCode: 400, Status: "Bad Request"},
		},
	}
	clause := r.dohClause()
	assert.Contains(t, clause, "拒绝了查询")
	assert.Contains(t, clause, "#type=", "应指出可操作的处置方式")
	assert.NotContains(t, clause, "不可达", "端点可达，不应说成不可达")
}

// TestDohClauseReportsUnreachable 断言真正的连接失败仍报为不可达。
func TestDohClauseReportsUnreachable(t *testing.T) {
	r := Report{
		DoH: DoHReport{
			Endpoint:  "https://1.1.1.1/dns-query",
			DirectErr: &net.OpError{Op: "dial", Err: errors.New("connection refused")},
		},
	}
	clause := r.dohClause()
	assert.Contains(t, clause, "不可达")
	assert.NotContains(t, clause, "#type=", "网络问题不应建议改编码方式声明")
}

// TestDohClauseReportsCertFailure 断言证书失败单独归因并指出处置方向。
func TestDohClauseReportsCertFailure(t *testing.T) {
	r := Report{
		DoH: DoHReport{
			Endpoint: "https://doh.example/dns-query",
			DirectErr: &tls.CertificateVerificationError{
				Err: errors.New("x509: certificate signed by unknown authority"),
			},
		},
	}
	clause := r.dohClause()
	assert.Contains(t, clause, "证书")
	assert.Contains(t, clause, "信任")
}

// TestDohClausePrefersDirectError 断言两条路径都失败时以直连的原因为准：
// 代理路径的失败可能只是代理不可用，直连的原因才说明端点本身的问题。
func TestDohClausePrefersDirectError(t *testing.T) {
	r := Report{
		Env: testEnv(true),
		DoH: DoHReport{
			DirectErr: &dns.StatusError{StatusCode: 400, Status: "Bad Request"},
			ProxyErr:  errors.New("代理不可用"),
		},
	}
	clause := r.dohClause()
	assert.Contains(t, clause, "拒绝了查询")
	assert.NotContains(t, clause, "代理不可用")
}

// TestStatusErrorFormatting 断言状态码错误带出状态码文本，
// 便于用户直接理解端点为何拒绝。
func TestStatusErrorFormatting(t *testing.T) {
	err := &dns.StatusError{StatusCode: 400, Status: "Bad Request"}
	assert.Equal(t, "status 400 Bad Request", err.Error())
}
