小说搜索
==================

地址: ``https://www.pixiv.net/ajax/search/novels/<标签>&p=<页数>``

一次返回 23-24 本。

数据格式:

[body.novel.data]

id: string 小说ID

title: string 标题

url: string 封面 URL

tags: string[] 标签

userId: string 作者ID

userName: string 作者名

textCount: number 字数

description

    类型: string

    作品描述HTML。

bookmarkCount: number 收藏数

seriesId

    类型: string | undefined

    系列ID

seriesTitle

    类型: string | undefined

    系列标题
