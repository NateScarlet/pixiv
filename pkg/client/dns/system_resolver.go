package dns

import (
	"context"
	"net"
)

type SystemResolver interface {
	Resolver
}

type systemResolver struct {
}

// Resolve implements SystemResolver
func (*systemResolver) Resolve(ctx context.Context, host string) (ip []net.IP, err error) {
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, len(addrs))
	for i, ia := range addrs {
		ips[i] = ia.IP
	}
	return ips, nil
}

func NewSystemResolver() SystemResolver {
	return &systemResolver{}
}
