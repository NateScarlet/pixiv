package client

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/NateScarlet/pixiv/pkg/client/dns"
)

// 默认值。环境变量是默认值的播种来源，用于「设置环境变量即生效」的既有部署方式。
const (
	defaultServerURL   = "https://www.pixiv.net"
	defaultDNSQueryURL = "https://1.1.1.1/dns-query"
	defaultUserAgent   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:84.0) Gecko/20100101 Firefox/84.0"
)

// Option 描述客户端的一项显式设置。
//
// 它是带未导出方法的接口而非函数类型，因此实现集合封闭在本包内，
// 调用者无法写出本包未预期的选项。
type Option interface {
	applyTo(*config)
}

// config 收集调用者的显式设置。
//
// 可空项以指针/存在性记录，从而区分「未设置」与「已设置为零值」：
// 只有未设置的项才会被默认值填充，因此显式设置总是胜出。
type config struct {
	transport     http.RoundTripper
	transportSet  bool
	dnsResolver   dns.Resolver
	dnsResolverSe bool
	phpSESSID     *string
	userAgent     *string
	serverURL     *string
}

type optionFunc func(*config)

func (f optionFunc) applyTo(c *config) { f(c) }

// WithTransport 用调用者提供的传输发送请求，优先于 DefaultTransport。
func WithTransport(rt http.RoundTripper) Option {
	return optionFunc(func(c *config) {
		c.transport = rt
		c.transportSet = true
	})
}

// WithDNSResolver 指定本库自行解析主机名时使用的解析器。
//
// 仅在使用本库自带的连接能力（例如图像主机的无 SNI 直连）时生效，
// 调用者自带的传输自行决定如何解析。
// 显式传入 nil 表示使用系统解析。
func WithDNSResolver(r dns.Resolver) Option {
	return optionFunc(func(c *config) {
		c.dnsResolver = r
		c.dnsResolverSe = true
	})
}

// WithPHPSESSID 用 PHPSESSID Cookie 登录。
func WithPHPSESSID(v string) Option {
	return optionFunc(func(c *config) { c.phpSESSID = &v })
}

// WithUserAgent 设置默认 User-Agent 请求头。
func WithUserAgent(v string) Option {
	return optionFunc(func(c *config) { c.userAgent = &v })
}

// WithServerURL 指定服务地址，用于测试与镜像场景。
func WithServerURL(v string) Option {
	return optionFunc(func(c *config) { c.serverURL = &v })
}

// New 依据选项构建客户端，未显式设置的项才由默认值填充。
//
// 装配只发生在这一处；构造过程不发起任何网络请求、不返回错误，
// 因此可用于包级变量初始化，失败在首次使用时以错误呈现。
func New(opts ...Option) *Client {
	var cfg config
	for _, i := range opts {
		if i == nil {
			continue
		}
		i.applyTo(&cfg)
	}

	var c = &Client{}
	if cfg.serverURL == nil {
		c.serverURL = defaultServerURL
	} else {
		c.serverURL = *cfg.serverURL
		if c.serverURL == "" {
			// 显式设置为空不是任何环境的描述，快速失败并指明出路。
			panic("pixiv: client: WithServerURL 的值为空: 未设置时省略该选项即可使用默认值")
		}
		// 服务地址在装配期解析校验一次，EndpointURL 运行时不再重复校验。
		if _, err := url.Parse(c.serverURL); err != nil {
			panic(fmt.Sprintf("pixiv: client: WithServerURL 的值 %q 无法解析: %v", c.serverURL, err))
		}
	}

	var userAgent string
	if cfg.userAgent == nil {
		userAgent = os.Getenv("PIXIV_USER_AGENT")
		if userAgent == "" {
			userAgent = defaultUserAgent
		}
	} else {
		// 显式设置为空表示不发送默认 User-Agent，而非回落默认值。
		userAgent = *cfg.userAgent
	}

	var phpSESSID string
	if cfg.phpSESSID == nil {
		phpSESSID = os.Getenv("PIXIV_PHPSESSID")
	} else {
		phpSESSID = *cfg.phpSESSID
		if phpSESSID == "" {
			panic("pixiv: client: WithPHPSESSID 的值为空: 未设置时省略该选项即可，显式设置为空不被允许")
		}
	}

	resolver := cfg.dnsResolver
	if !cfg.dnsResolverSe {
		resolver = defaultDNSResolver()
	}

	if cfg.transportSet {
		if cfg.transport == nil {
			panic("pixiv: client: WithTransport 的值为空: 未设置时省略该选项即可使用 DefaultTransport")
		}
		c.Transport = cfg.transport
	} else {
		if DefaultTransport == nil {
			panic("pixiv: client: DefaultTransport 被置空而调用者未提供传输: 替换 DefaultTransport 时不能设为 nil")
		}
		c.Transport = DefaultTransport
	}
	if resolver != nil {
		// 解析器随请求传递，供自行拨号的连接能力取用，不作为 Client 的字段，
		// 因此传输无需反向引用 Client、Client 也不承载同步状态。
		c.Transport = &resolverTransport{wrapped: c.Transport, resolver: resolver}
	}

	if userAgent != "" {
		c.setDefaultHeader("User-Agent", userAgent)
	}
	if phpSESSID != "" {
		c.setPHPSESSID(phpSESSID)
	}
	return c
}

// defaultDNSResolver 返回环境变量播种的默认解析器，供本库自带的
// 连接能力（例如图像主机的无 SNI 直连）解析目标主机。
func defaultDNSResolver() dns.Resolver {
	var queryURL = os.Getenv("PIXIV_DNS_QUERY_URL")
	if queryURL == "" {
		queryURL = defaultDNSQueryURL
	}
	return dns.NewCache(
		dns.NewDOHResolver(queryURL),
		time.Hour,
	)
}
