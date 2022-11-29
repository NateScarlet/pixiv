package dns

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDOHResolver(t *testing.T) {
	for _, url := range []string{
		"https://1.1.1.1/dns-query",
		"https://1.0.0.1/dns-query",
		"https://dns.nextdns.io",
	} {
		t.Run(url, func(t *testing.T) {
			r := NewDOHResolver(url)
			ip, err := r.Resolve(context.Background(), "www.pixiv.net")
			require.NoError(t, err)
			assert.NotEmpty(t, ip)
			assert.Regexp(t, `\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`, ip[0].String())
		})
	}

}
