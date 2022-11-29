package dns

import (
	"context"
	"net"
)

type HostReplacer interface {
	Resolver
}

type hostReplacer struct {
	raw Resolver

	m map[string]string
}

// Resolve implements HostReplacer
func (r *hostReplacer) Resolve(ctx context.Context, host string) (ip []net.IP, err error) {
	if v, ok := r.m[host]; ok {
		host = v
	}
	return r.raw.Resolve(ctx, host)
}

func NewHostReplacer(raw Resolver, oldNew ...string) HostReplacer {
	if len(oldNew)%2 == 1 {
		panic("dns.NewHostReplacer: odd argument count")
	}
	var m = make(map[string]string, len(oldNew)/2)
	for i := 0; i < len(oldNew); i += 2 {
		m[oldNew[i]] = oldNew[i+1]
	}
	return &hostReplacer{
		raw: raw,
		m:   m,
	}
}
