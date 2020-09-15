package client

import (
	"io/ioutil"
	"net/http"

	"github.com/tidwall/gjson"
)

// Hosts cache
var Hosts = map[string]string{}

// TODO: support custom dns server
func resolveHostname(hostname string) (ip string, err error) {
	if v, ok := Hosts[hostname]; ok {
		return v, nil
	}

	resp, err := http.Get("https://1.1.1.1/dns-query?ct=application/dns-json&type=A&name=" + hostname)
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

func init() {
	Hosts["www.pixiv.net"], _ = resolveHostname("pixiv.net") // www.pixiv.net resolve to a blocked ip
}
