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
	// client 发出查询请求；nil 时使用 http.DefaultClient（遵循进程代理环境变量）。
	client *http.Client
}

func (r *dohResolver) httpClient() *http.Client {
	if r.client == nil {
		return http.DefaultClient
	}
	return r.client
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

	resp, err := r.httpClient().Do(req)
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
	return &dohResolver{url: url, client: nil}
}

// NewDOHResolverWithClient 用指定的 HTTP client 构造 DoH 解析器。
//
// 运行时用 [NewDOHResolver] 即可：其查询经 http.DefaultClient 发出，
// 遵循进程代理环境变量（HTTPS_PROXY 等）。需要受控代理行为的调用者
// （例如诊断工具对照「经代理 / 直连」两种查询路径）用本构造函数注入
// 自己的 client；注入的 client 零值字段按标准库默认处理。
func NewDOHResolverWithClient(url string, client *http.Client) DOHResolver {
	return &dohResolver{url: url, client: client}
}
