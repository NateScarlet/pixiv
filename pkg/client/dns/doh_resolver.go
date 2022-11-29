package dns

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"

	"github.com/tidwall/gjson"
)

type DOHResolver interface {
	Resolver
	URL() string
}

type dohResolver struct {
	url string
}

// Resolve implements DNSResolver
func (r *dohResolver) Resolve(ctx context.Context, name string) (ip []net.IP, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("dohResolver{'%s'}.Resolve('%s'): %w", r.url, name, err)
		}
	}()

	req, err := http.NewRequestWithContext(ctx, "GET", r.url, nil)
	if err != nil {
		return
	}
	var q = req.URL.Query()
	q.Set("name", name)
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Accept", "application/dns-json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("status %d", resp.StatusCode)
		return
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}
	var jsonData = gjson.ParseBytes(data)
	jsonData.Get("Answer.#(type==1)#.data").ForEach(func(key, value gjson.Result) bool {
		ip = append(ip, net.ParseIP(value.String()))
		return true
	})
	return
}

// URL implements DOHResolver
func (r *dohResolver) URL() string {
	return r.url
}

func NewDOHResolver(url string) DOHResolver {
	return &dohResolver{url}
}
