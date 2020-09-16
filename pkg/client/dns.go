package client

import (
	"io/ioutil"
	"net/http"
	"os"

	"github.com/tidwall/gjson"
)

func lookupDNSOverHTTPS(dnsQueryURL string, hostname string) (ip string, err error) {
	req, err := http.NewRequest("GET", dnsQueryURL, nil)
	if err != nil {
		return
	}
	var q = req.URL.Query()
	q.Set("name", hostname)
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Accept", "application/dns-json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}
	var jsonData = gjson.ParseBytes(data)
	ip = jsonData.Get("Answer.#(type==1).data").String()
	return
}

// Hosts rules like /etc/hosts
var Hosts = map[string]string{}

// DNSQueryURL is DNS over HTTPS service url that act like google public dns json api
// https://developers.google.com/speed/public-dns/docs/doh/json
var DNSQueryURL string = os.Getenv("PIXIV_DNS_QUERY_URL")

func init() {
	if DNSQueryURL == "" {
		DNSQueryURL = "https://1.1.1.1/dns-query"
		// DNSQueryURL = "https://1.0.0.1/dns-query"
		// DNSQueryURL = "https://cloudflare-dns.com/dns-query"
		// DNSQueryURL = "https://dns.nextdns.io/dns-query"
	}
}

func resolveHostname(hostname string) (ip string, err error) {
	if v, ok := Hosts[hostname]; ok && v != "" {
		return v, nil
	}

	return lookupDNSOverHTTPS(DNSQueryURL, hostname)
}

func init() {
	Hosts["www.pixiv.net"], _ = resolveHostname("pixiv.net") // www.pixiv.net resolve to a blocked ip
}
