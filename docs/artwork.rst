
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

    类型: 0 | 1 | 2

    插画类型， 0: 插画 1: 漫画 2: 动图。

description

    类型: string

    作品描述HTML。

urls

    类型: Record<'mini' | 'thumb' | 'small' | 'regular' | 'original" , string>

    各个尺寸的图像地址。


createDate

    类型: string, rfc3339 日期

    创建时间

uploadDate

    类型: string, rfc3339 日期

    上传时间，有可能晚于创建时间或是一样的。

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

aiType

    类型: 0 | 1 | 2

    插画类型， 0: 未知 1: 非AI生成 2: AI生成。

[seriesNavData]: null | object

    作品所属系列数据， 作品不属于任何系列时该字段缺失。

seriesNavData.seriesId

    类型: number

    系列 ID

seriesNavData.title

    类型: string

    系列标题

seriesNavData.order

    类型: number

    作品在系列中的次序， 从 1 开始

[seriesNavData.prev]: null | object

    系列中紧邻的上一章节， 无上一章节时为 null。

seriesNavData.prev.id

    类型: string

    上一章节的作品 ID

seriesNavData.prev.title

    类型: string

    上一章节标题

seriesNavData.prev.order

    类型: number

    上一章节在系列中的次序

seriesNavData.prev.available

    类型: boolean

    上一章节是否可访问

[seriesNavData.next]: null | object

    系列中紧邻的下一章节， 字段同 prev。

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
