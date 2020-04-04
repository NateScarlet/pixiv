# pixiv client for go

[![build status](https://github.com/NateScarlet/pixiv/workflows/Go/badge.svg)](https://github.com/NateScarlet/pixiv/actions)

Pixiv go 客户端， 使用 PIXIV 网页 API。

[设计文档](https://natescarlet.github.io/pixiv/)

详细使用方法以代码注释为准

```go
package main

import (
    "github.com/NateScarlet/pixiv/pkg/client"
    "github.com/NateScarlet/pixiv/pkg/artwork"
    "github.com/NateScarlet/pixiv/pkg/novel"
    "github.com/NateScarlet/pixiv/pkg/user"
)

// 默认客户端用环境变量 `PIXIV_PHPSESSID` 登录。
// 默认使用默认客户端, 所有查询都有 *WithClient 版本指定客户端。
client.Default

// 通过账号密码登录(可能触发 reCAPTCHA)。
c := &client.Client{}
c.Login("username", "password")

// 搜索画作
result, err := artwork.Search("パチュリー・ノーレッジ", 1)
result.JSON // json return data.
result.Artworks() // []artwork.Artwork，只有部分数据，通过 `Fetch` `FetchPages` 方法获取完整数据。

// 画作详情
i := &artwork.Artwork{ID: "22238487"}
err := i.Fetch() // 获取画作详情(不含分页), 直接更新 struct 数据。
err := i.FetchPages() // 获取画作分页, 直接更新 struct 数据。

// 画作排行榜
rank := &artwork.Rank{Mode: "daily"}
rank.Fetch()
rank.Items[0].Rank
rank.Items[0].PreviousRank
rank.Items[0].Artwork

// 搜索小说
result, err := novel.Search("パチュリー・ノーレッジ", 1)
result.JSON // json return data.
result.Novels() // []novel.Novel，只有部分数据，通过 `Fetch` 方法获取完整数据。

// 小说详情
i := &novel.Novel{ID: "11983096"}
err := i.Fetch() // 获取小说详情, 直接更新 struct 数据。

// 用户详情
i := &user.User{ID: "789096"}
err := i.Fetch() // 获取用户详情, 直接更新 struct 数据。
```
