package dns

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

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
	// ownProxyHosts 是自身访问链路依赖的地址（经自身查询的代理等），按系统
	// 解析处理，见 Resolve 的递归说明。
	ownProxyHosts map[string]struct{}
}

func (r *dohResolver) httpClient() *http.Client {
	if r.client == nil {
		return http.DefaultClient
	}
	return r.client
}

// Resolve implements DNSResolver
//
// 递归说明：DoH 查询自身的出网可能依赖代理（查询经 http.DefaultClient，
// 遵循 HTTPS_PROXY），而传输层的解析接缝会把代理地址也交给解析器。
// 若对这些名字再用 DoH 解析，就构成「解析代理 → 需经代理 → 代理地址待解析」
// 的自举依赖。因此解析自身访问链路依赖的代理主机名时直接走系统解析，
// 不发出 DoH 查询。
func (r *dohResolver) Resolve(ctx context.Context, name string) (ip []net.IP, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("dohResolver{'%s'}.Resolve('%s'): %w", r.url, name, err)
		}
	}()

	if _, own := r.ownProxyHosts[strings.ToLower(name)]; own {
		addrs, err := net.DefaultResolver.LookupIPAddr(ctx, name)
		if err != nil {
			return nil, err
		}
		ips := make([]net.IP, 0, len(addrs))
		for _, a := range addrs {
			ips = append(ips, a.IP)
		}
		return ips, nil
	}

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
	return &dohResolver{url: url, client: nil, ownProxyHosts: ownProxyHostsFromEnv()}
}

// NewDOHResolverWithClient 用指定的 HTTP client 构造 DoH 解析器。
//
// 运行时用 [NewDOHResolver] 即可：其查询经 http.DefaultClient 发出，
// 遵循进程代理环境变量（HTTPS_PROXY 等）。需要受控代理行为的调用者
// （例如诊断工具对照「经代理 / 直连」两种查询路径）用本构造函数注入
// 自己的 client；注入的 client 零值字段按标准库默认处理。
//
// 两个构造函数都会把进程代理环境变量指向的主机名登记为自身访问链路的
// 依赖，见 [dohResolver.Resolve] 的递归说明。
func NewDOHResolverWithClient(url string, client *http.Client) DOHResolver {
	return &dohResolver{url: url, client: client, ownProxyHosts: ownProxyHostsFromEnv()}
}

// ownProxyHostsFromEnv 取进程代理环境变量（与 http.ProxyFromEnvironment
// 的取值集合一致）中各代理地址的主机名部分。代理是 DoH 查询自身的出网
// 途径，对这些名字的解析是解析器的自举依赖。
func ownProxyHostsFromEnv() map[string]struct{} {
	var hosts map[string]struct{}
	for _, key := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"} {
		raw := os.Getenv(key)
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" {
			continue
		}
		if hosts == nil {
			hosts = make(map[string]struct{})
		}
		hosts[strings.ToLower(u.Hostname())] = struct{}{}
	}
	return hosts
}
