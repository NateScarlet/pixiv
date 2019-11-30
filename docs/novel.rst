
小说详情
====================

[body]

id: string 小说ID

title: string 标题

userId: string 作者ID

userName: string 作者名

description: string HTML描述

createDate

    类型: string, rf3339 日期

    创建时间

uploadDate

    类型: string, rf3339 日期

    上传时间，有可能晚于创建时间是一样的。

coverUrl

    类型: string

    小说封面

content

    类型: string

    小说内容。

    ``[newpage]`` 代表新一页。

    ``[pixivimage:{图像编号}]`` 代表插入站内画作。 图像编号格式为 ``{ID}(-{index})?``


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

小说插图
============

地址: ``https://www.pixiv.net/ajax/novel/<小说ID>/insert_illusts``

参数:

id[]

    要查询的图像编号，可重复使用。
