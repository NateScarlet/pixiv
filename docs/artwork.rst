
画作详情
==================

API 地址格式: ``https://www.pixiv.net/ajax/illust/<作品ID>``

[body]

illustId

    类型: string | null

    作品 Id。

illustTitle

    类型: string

    作品标题。

illustType

    类型: "0" | "1" | "2"

    插画类型， 0: 插画 1: 漫画 2: 动图。

description

    类型: string

    作品描述HTML。

urls

    类型: Record<'mini' | 'thumb' | 'small' | 'regular' | 'original" , string>

    各个尺寸的图像地址。


createDate

    类型: string, rf3339 日期

    创建时间

uploadDate

    类型: string, rf3339 日期

    上传时间，有可能晚于创建时间是一样的。

userId

    类型: string

    作者 Id。

userName

    类型: string

    作者名称。

pageCount

    类型: number

    页数。

commentCount

    类型: number

    回复数。

likeCount

    类型: number

    赞数

viewCount

    类型: number

    浏览量

bookmarkCount

    收藏数

[body.illustManga.tags.tags]

    标签数据列表

tag

    类型: string

    标签名称

画作分页详情
=================

直接查询画作返回的图像 URL 是第一页的。

地址: ``https://www.pixiv.net/ajax/illust/<ID>/pages``

[body]

[body.urls]

thumb_mini

    类型: string

    128px 图像地址。

small

    类型: string

    540px 图像地址。

regular

    类型: string

    1200px 图像地址。

original

    类型: string

    原始图像地址。
