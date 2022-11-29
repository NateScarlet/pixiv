package dns

import (
	"context"
	"net"
	"sync"
	"time"
)

type Cache interface {
	Resolver
}

type cacheValue struct {
	ip      []net.IP
	fetched time.Time
}

type cache struct {
	raw    Resolver
	mu     sync.Mutex
	m      map[string]cacheValue
	maxAge time.Duration
}

// Resolve implements Cache
func (c *cache) Resolve(ctx context.Context, host string) (ip []net.IP, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v := c.m[host]
	if time.Since(v.fetched) > c.maxAge {
		rawIP, rawErr := c.raw.Resolve(ctx, host)
		if rawErr != nil && len(v.ip) == 0 {
			// use stale result
			err = rawErr
			return
		}
		if rawErr == nil {
			v.ip = rawIP
			v.fetched = time.Now()
		}
	}
	c.m[host] = v
	return v.ip, nil
}

func NewCache(raw Resolver, maxAge time.Duration) Cache {
	return &cache{raw: raw, maxAge: maxAge, m: make(map[string]cacheValue)}
}
