package client

import (
	"os"
	"time"

	"github.com/NateScarlet/pixiv/pkg/client/dns"
)

// DNSQueryURL is DNS over HTTPS service url that act like google public dns json api
// https://developers.google.com/speed/public-dns/docs/doh/json
var DefaultDNSQueryURL = os.Getenv("PIXIV_DNS_QUERY_URL")

// DefaultUserAgent for new clients
var DefaultUserAgent = os.Getenv("PIXIV_USER_AGENT")

var DefaultPHPSESSID = os.Getenv("PIXIV_PHPSESSID")
var DefaultBypassSNIBlocking = os.Getenv("PIXIV_BYPASS_SNI_BLOCKING") != ""

// Default client auto login with PIXIV_PHPSESSID env var.
var Default *Client

func init() {
	if DefaultUserAgent == "" {
		DefaultUserAgent = `Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:84.0) Gecko/20100101 Firefox/84.0`
	}
	if DefaultDNSQueryURL == "" {
		DefaultDNSQueryURL = "https://1.1.1.1/dns-query"
	}
	Default = NewDefaultClient()
}

func (c *Client) ApplyDefault() {
	if DefaultBypassSNIBlocking {
		c.BypassSNIBlocking()
	}
	if DefaultDNSQueryURL != "" {
		c.DNSResolver = dns.NewHostReplacer(
			dns.NewCache(
				dns.NewDOHResolver(DefaultDNSQueryURL),
				time.Hour,
			),
			// www.pixiv.net may resolve to a blocked ip
			"www.pixiv.net",
			"pixiv.net",
		)
	}
	if DefaultPHPSESSID != "" {
		c.SetPHPSESSID(DefaultPHPSESSID)
	}
	if DefaultUserAgent != "" {
		c.SetDefaultHeader("User-Agent", DefaultUserAgent)
	}

}

func NewDefaultClient() *Client {
	var v = new(Client)
	v.ApplyDefault()
	return v
}
