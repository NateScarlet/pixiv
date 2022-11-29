package dns

import (
	"context"
	"net"
)

type Resolver interface {
	Resolve(ctx context.Context, host string) (ip []net.IP, err error)
}
