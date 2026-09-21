# pixiv client for go

[![godev](https://img.shields.io/static/v1?label=godev&message=reference&color=00add8)](https://pkg.go.dev/github.com/NateScarlet/pixiv/pkg)
[![build status](https://github.com/NateScarlet/pixiv/workflows/Go/badge.svg)](https://github.com/NateScarlet/pixiv/actions)

Pixiv go 客户端， 使用 PIXIV 网页 API。

[设计文档](https://natescarlet.github.io/pixiv/)

- [x] 画作搜索
- [x] 画作排行榜
- [x] 画作详情
- [x] 系列数据 (画作/小说详情)
- [x] 小说搜索
- [ ] 小说排行榜
- [x] 小说详情
- [x] 渲染小说为 HTML
- [ ] 用户详情

详细使用方法以代码注释为准

2024-08-27: 账号密码登录方式已失效，手动登录获取 PHPSESSID 代替

novel 包旧版建模 API（`Search`/`SearchResult`/`Novel.Fetch`）已标记为 Deprecated，请改用 `SearchV2`/`Fetch`（不可变记录）。

```go
package main

import (
    "context"
    "fmt"
    "io"
    "net/http"
    "os"
    "slices"

    "github.com/NateScarlet/pixiv/pkg/client"
    "github.com/NateScarlet/pixiv/pkg/client/dns"
    "github.com/NateScarlet/pixiv/pkg/artwork"
    "github.com/NateScarlet/pixiv/pkg/novel"
    "github.com/NateScarlet/pixiv/pkg/user"
)

// 默认客户端用环境变量 `PIXIV_PHPSESSID` 登录。
// 并且 User-Agent 使用 `PIXIV_USER_AGENT` 或库内置的默认值。
client.Default

// 用选项构建客户端：显式设置的项胜出，未设置的项才由默认值填充。
// 装配只发生在 New 一处，构造过程不发任何请求。
c := client.New(
    client.WithPHPSESSID("PHPSESSID"), // 用 PHPSESSID Cookie 登录 (推荐)
    client.WithUserAgent("Mozilla/5.0 ..."),
)

// 指定服务地址 (测试或镜像场景)。
mirror := client.New(client.WithServerURL("https://mirror.example.com"))

// 注入自己的 Transport 以使用代理或任何自定义管道; 优先级高于 DefaultTransport。
proxied := client.New(client.WithTransport(&http.Transport{
    Proxy: http.ProxyURL(proxyURL),
}))

// 需要自行控制传输层时, 用库提供的传输原语组合管道。
// 原语不含主机判断: 单独使用时由调用者决定何时用它。
noSNI := client.New(client.WithTransport(client.NewNoSNITransport(&http.Transport{
    Proxy: http.ProxyURL(proxyURL),
})))

// 或者让库按主机自动选用合适的方式 (主机清单由库持有, 随依赖升级更新)。
// 默认传输即为它: 托管在 Cloudflare 的 API 主机会经 ECH 直连, 失败时回落。
routed := client.New(client.WithTransport(client.NewRoutedTransport(&http.Transport{
    Proxy: http.ProxyURL(proxyURL),
})))

// 需要单独控制时, 也可以只用 ECH 原语 (外层 SNI 为 cloudflare-ech.com)。
// 未提供配置时原语自行取得, 配置轮换时自行恢复; 不适用于 pixiv 自有源站。
// ECH 主机的数据连接始终直连, 即使这里配置了代理也不会经它发出。
ech := client.New(client.WithTransport(client.NewECHTransport(&http.Transport{})))

// 直连时若系统解析返回被污染的地址, 配合可用的 DoH 解析器。
echDirect := client.New(
    client.WithTransport(client.NewECHTransport(&http.Transport{})),
    client.WithDNSResolver(dns.NewDOHResolver("https://1.1.1.1/dns-query")),
)

// 注入自己的解析器; 它只被库自带的连接能力使用。
resolved := client.New(
    client.WithDNSResolver(dns.NewDOHResolver("https://1.1.1.1/dns-query")),
)

// 所有查询从 context 获取客户端设置, 如未设置将使用默认客户端。
var ctx = context.Background()
ctx = client.With(ctx, c)

// 搜索画作 (默认最新排序)
payload, _ := artwork.SearchV2(ctx, "パチュリー・ノーレッジ")
for item := range payload.Items() {
    fmt.Println(item.Title(), "by", item.AuthorName())
}

// 高级搜索 (带过滤选项)
payload, _ := artwork.SearchV2(ctx, "パチュリー・ノーレッジ",
    artwork.SearchWithPage(3),              // 第3页
    artwork.SearchWithContentRating(artwork.R18Content),
    artwork.SearchWithMode(artwork.PartialTagSearch)
)

// 获取排行榜
rank, _ := artwork.FetchRank(ctx,
    artwork.DailyRank,                     // 每日榜
    artwork.FetchRankWithDate(yesterday),  // 指定日期
    artwork.FetchRankWithPage(2)           // 第二页
)
for item := range rank.Items() {
    fmt.Printf("第%d名: %s", item.Position(), item.Title())
}

// 获取画作元数据
art, _ := artwork.Fetch(ctx, "22238487")
fmt.Println("作品描述:", art.Description())
for tag := range art.Tags() {
    fmt.Println("标签:", tag)
}
fmt.Println("查看网页版:", art.URL().String())
if series := art.Series(); !series.IsZero() {
    fmt.Println("所属系列:", series.Title(), "第", series.Order(), "话")
}

// 获取画作全部分页
pages, _ := artwork.FetchPages(ctx, "22238487")
for page := range pages.Pages() {
    fmt.Println("原图地址:", page.OriginalURL())
}

// 取回图片内容; 方法自动带上必需的 Referer, 并按主机把请求交给合适的传输。
// 它只返回响应、不落盘: 写文件、解码、算哈希都由调用者自行决定。
for i, page := range slices.Collect(pages.Pages()) {
    resp, err := client.For(ctx).FetchImage(ctx, page.OriginalURL())
    if err != nil {
        fmt.Println("取回失败:", err)
        break
    }
    // 格式从响应读取, 不要按 URL 扩展名推断 (原图尤其如此)。
    fmt.Println("格式:", resp.Header.Get("Content-Type"))
    // 可直接流式写出, 不必先把大图读进内存。
    f, err := os.Create(fmt.Sprintf("22238487_p%d.png", i))
    if err != nil {
        resp.Body.Close()
        fmt.Println("创建文件失败:", err)
        break
    }
    _, err = io.Copy(f, resp.Body)
    resp.Body.Close()
    f.Close()
    if err != nil {
        fmt.Println("写入失败:", err)
        break
    }
}

// 搜索小说 (不可变记录, 已知字段方法 + Raw() 获取未建模字段)
payload, _ := novel.SearchV2(ctx, "パチュリー・ノーレッジ")
for item := range payload.Items() {
    fmt.Println(item.Title(), "by", item.AuthorName())
}

// 小说详情 (不可变记录)
data, _ := novel.Fetch(ctx, "11983096")
fmt.Println("标题:", data.Title())
series := data.Series()
if !series.IsZero() {
    fmt.Println("所属系列:", series.Title(), "第", series.Order(), "话")
}

// 用户详情
i := &user.User{ID: "789096"}
err := i.Fetch(ctx) // 获取用户详情, 直接更新 struct 数据。
```
