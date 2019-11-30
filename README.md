# pixiv client for go

Pixiv go 客户端， 使用 PIXIV 网页 API。

```go
package main

import (
    "github.com/NateScarlet/pixiv/pkg/client"
)

// 通过环境变量 `PIXIV_USER`+`PIXIV_PASSWORD` 或 `PIXIV_PHPSESSID` 登录。
client.LoginFromEnv()

// 通过账号密码登录(可能触发 reCAPTCHA)
client.Login("username", "password")

// 搜索画作
result, err := client.SearchArtwork("パチュリー・ノーレッジ", 1)
result.JSON // json return data.
result.Artworks() // []client.Artwork，只有部分数据，通过 `Fetch` `FetchPages` 方法获取完整数据。

// 画作详情
i := Artwork{ID: "22238487"}
err := i.Fetch() // 获取画作详情(不含分页), 直接更新 struct 数据。
err := i.FetchPages() // 获取画作分页, 直接更新 struct 数据。

// 搜索小说
result, err := client.SearchNovel("パチュリー・ノーレッジ", 1)
result.JSON // json return data.
result.Novel() // []client.Novel，只有部分数据，通过 `Fetch` 方法获取完整数据。

// 小说详情
i := Novel{ID: "11983096"}
err := i.Fetch() // 获取小说详情, 直接更新 struct 数据。

// 用户详情
i := User{ID: "789096"}
err := i.Fetch() // 获取用户详情, 直接更新 struct 数据。
```
