package client

import (
	"context"
	"net/http"

	"github.com/NateScarlet/pixiv/pkg/client/dns"
)

type dnsResolverContextKey struct{}

// dnsResolverFromContext 取出请求中注入的解析器，未注入时返回 nil。
func dnsResolverFromContext(ctx context.Context) dns.Resolver {
	v, _ := ctx.Value(dnsResolverContextKey{}).(dns.Resolver)
	return v
}

func withDNSResolver(ctx context.Context, r dns.Resolver) context.Context {
	return context.WithValue(ctx, dnsResolverContextKey{}, r)
}

// resolverTransport 把解析器注入请求上下文。
//
// 本库自带的连接能力（例如图像主机的无 SNI 直连）在拨号时从上下文取用解析器，
// 从而不需要反向引用 Client、也不给 Client 增加同步状态。
type resolverTransport struct {
	wrapped  http.RoundTripper
	resolver dns.Resolver
}

// RoundTrip implements http.RoundTripper
func (t *resolverTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.wrapped.RoundTrip(req.WithContext(withDNSResolver(req.Context(), t.resolver)))
}
