# pixiv client for go

[![godev](https://img.shields.io/static/v1?label=godev&message=reference&color=00add8)](https://pkg.go.dev/github.com/NateScarlet/pixiv/pkg)
[![build status](https://github.com/NateScarlet/pixiv/workflows/Go/badge.svg)](https://github.com/NateScarlet/pixiv/actions)

Pixiv go 客户端， 使用 PIXIV 网页 API。

[设计文档](https://natescarlet.github.io/pixiv/)

- [x] 画作搜索
- [x] 画作排行榜
- [x] 画作详情
- [x] 小说搜索
- [ ] 小说排行榜
- [x] 小说详情
- [x] 渲染小说为 HTML
- [ ] 用户详情

详细使用方法以代码注释为准

2024-08-27: 账号密码登录方式已失效，手动登录获取 PHPSESSID 代替

```go
package main

import (
    "context"

    "github.com/NateScarlet/pixiv/pkg/client"
    "github.com/NateScarlet/pixiv/pkg/artwork"
    "github.com/NateScarlet/pixiv/pkg/novel"
    "github.com/NateScarlet/pixiv/pkg/user"
)

// 默认客户端用环境变量 `PIXIV_PHPSESSID` 登录。
// 并且 User-Agent 使用 `PIXIV_USER_AGENT` 或库内置的默认值。
client.Default

// 使用 PHPSESSID Cookie 登录 (推荐)。
c := &client.Client{}
c.SetDefaultHeader("User-Agent", client.DefaultUserAgent)
c.SetPHPSESSID("PHPSESSID")

// 启用免代理，环境变量 `PIXIV_BYPASS_SNI_BLOCKING` 不为空时自动为默认客户端启用免代理。
// 当前实现需求一个 DNS over HTTPS 服务，默认使用 cloudflare，可通过 `PIXIV_DNS_QUERY_URL` 环境变量设置。
// 必须在其他客户端选项前调用 `BypassSNIBlocking`，因为对于封锁的域名它会使用一个更改过的 Transport 进行请求，无视在它之前进行的的设置。
c := &client.Client{}
c.BypassSNIBlocking()
c.SetDefaultHeader("User-Agent", client.DefaultUserAgent)

// 所有查询从 context 获取客户端设置, 如未设置将使用默认客户端。
var ctx = context.Background()
ctx = client.With(ctx, c)

// 搜索画作
result, err := artwork.Search(ctx, "パチュリー・ノーレッジ")
result.JSON // json return data.
result.Artworks() // []artwork.Artwork，只有部分数据，通过 `Fetch` `FetchPages` 方法获取完整数据。
artwork.Search(ctx, "パチュリー・ノーレッジ", artwork.SearchOptionPage(2)) // 获取第二页

// 画作详情
i := &artwork.Artwork{ID: "22238487"}
err := i.Fetch(ctx) // 获取画作详情(不含分页), 直接更新 struct 数据。
err := i.FetchPages(ctx) // 获取画作分页, 直接更新 struct 数据。

// 画作排行榜
rank := &artwork.Rank{Mode: "daily"}
rank.Fetch(ctx)
rank.Items[0].Rank
rank.Items[0].PreviousRank
rank.Items[0].Artwork

// 搜索小说
result, err := novel.Search(ctx, "パチュリー・ノーレッジ")
result.JSON // json return data.
result.Novels() // []novel.Novel，只有部分数据，通过 `Fetch` 方法获取完整数据。
novel.Search(ctx, "パチュリー・ノーレッジ", novel.SearchOptionPage(2)) // 获取第二页

// 小说详情
i := &novel.Novel{ID: "11983096"}
err := i.Fetch(ctx) // 获取小说详情, 直接更新 struct 数据。

// 用户详情
i := &user.User{ID: "789096"}
err := i.Fetch(ctx) // 获取用户详情, 直接更新 struct 数据。
```
