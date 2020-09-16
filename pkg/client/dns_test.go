package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLookupDNSOverHTTPS(t *testing.T) {
	for _, url := range []string{
		"https://1.0.0.1/dns-query",
		"https://1.1.1.1/dns-query",
		"https://dns.nextdns.io",
	} {
		ip, err := lookupDNSOverHTTPS(url, "pixiv.net")
		require.NoError(t, err)
		assert.Regexp(t, `\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`, ip)
	}

}
