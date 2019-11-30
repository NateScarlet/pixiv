画作搜索
==================

API 地址格式: ``https://www.pixiv.net/ajax/search/artworks/<标签>``

API 参数:

p

  页号


数据格式:

[body.illustManga]

isAdContainer

    类型: true | undefined | false

    是否为广告容器。

    2019-06-16: 40 张图， 其中可能有 1 广告。

    2019-11-20: 改成 API 调用了，60 张图其中可能有 1 广告。

illustId

    类型: string | null

    插画 Id。

illustTitle

    类型: string

    插画标题。

illustType

    类型: "0" | "1" | "2"

    插画类型， 0: 插画 1: 漫画 2: 动图。

url

    类型: string

    缩略图地址。

tags

    类型: string[]

    标签。

userId

    类型: string

    作者 Id。

userName

    类型: string

    作者名称。

profileImageUrl

    类型: string

    作者头像地址

isBookmarkable

    类型: boolean | null

    是否可收藏，未登录时为 null。

bookmarkData

    类型: object | null

    当前用户的收藏状况，未登录时为 null。

bookmarkData.id

    类型: string

    收藏 ID。

bookmarkData.private

    类型: boolean

    是否为非公开收藏。

width

    类型: number

    图片原始宽度。

height

    类型: height

    图片原始高度。

pageCount

    类型: number

    页数。

profileImageUrl

    类型: string

    用户头像地址。

[body.popular]

[[body.popular.permanent]]

    类型同 [body.illustManga]

[[body.popular.recent]]

    类型同 [body.illustManga]
