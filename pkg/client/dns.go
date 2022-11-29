package client

import (
	"github.com/NateScarlet/pixiv/pkg/client/dns"
)

func (c Client) ensureDNSResolver() dns.Resolver {
	if c.DNSResolver == nil {
		return dns.NewSystemResolver()
	}
	return c.DNSResolver
}
