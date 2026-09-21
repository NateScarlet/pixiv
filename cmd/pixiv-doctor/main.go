// Command pixiv-doctor 检测当前环境对本库各连接方式的可达性，输出报告。
//
// 报告回答三件事：
//
//  1. 环境多大程度上可以直连（ECH / 无 SNI / 常规）；
//  2. AutoTransport（库的默认传输）是否可链接；
//  3. 配置了 HTTPS_PROXY 时，DoH 查询与数据传输（API / 图片）是否需要经过代理。
//
// 用法:
//
//	pixiv-doctor
//
// 环境变量与库一致：PIXIV_DNS_QUERY_URL 选择 DoH 端点，
// HTTPS_PROXY 声明代理（其存在会触发额外的代理路径探测）。
package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"runtime"

	"github.com/NateScarlet/pixiv/internal/connectivity"
	"github.com/NateScarlet/pixiv/pkg/client"
	"github.com/NateScarlet/pixiv/pkg/client/dns"
)

// main 是最外层装配：读取环境、构建探测器与套件、渲染报告。
//
// 报告写到 stdout；探测正常完成但结论为不可用时进程仍以 0 退出
// ——诊断输出本身是成功的结果，退出码供调用方区分的是工具自身是否运行。
func main() {
	env := buildEnvironment()
	resolver := dns.NewDOHResolver(env.DoHQueryURL)
	suite := connectivity.Suite{}
	prober := connectivity.NewLiveProber(env.ProxyURL, resolver)

	report := suite.Run(context.Background(), env, prober)
	if err := connectivity.Render(os.Stdout, report); err != nil {
		fmt.Fprintln(os.Stderr, "pixiv-doctor:", err)
		os.Exit(1)
	}
}

// buildEnvironment 收集环境信息。
//
// HTTPS_PROXY 的取值与运行时一致（ProxyFromEnvironment 遵循
// HTTPS_PROXY / https_proxy 等变量）：这里显式读取 HTTPS_PROXY 作为声明来源，
// 与文档约定一致；解析失败快速失败，带原始值退出。
func buildEnvironment() connectivity.Environment {
	env := connectivity.Environment{
		GoVersion: runtime.Version(),
	}

	if raw := os.Getenv("HTTPS_PROXY"); raw != "" {
		env.HTTPSProxyRaw = raw
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			fmt.Fprintf(os.Stderr, "pixiv-doctor: HTTPS_PROXY 的值 %q 不是有效的代理地址\n", raw)
			os.Exit(1)
		}
		env.ProxyURL = u
	}

	env.DoHQueryURL = os.Getenv("PIXIV_DNS_QUERY_URL")
	if env.DoHQueryURL == "" {
		env.DoHQueryURL = client.DefaultDNSQueryURL
		env.DoHQueryURLIsDefault = true
	}
	return env
}
