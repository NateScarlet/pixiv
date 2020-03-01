画作排行
===============

页面URL: https://www.pixiv.net/ranking.php

请求参数：

mode 

  排行模式，必填。

  可选值：

    - daily

    - weekly

    - monthly

    - rookie

    - original
  
    - male

    - female

    - daily_r18

    - weekly_r18

    - male_r18

    - female_r18

    - r18g

content

  排行内容，不填则为综合排行，有些排行模式只对综合排行有效。

  可选值:

    - illust

    - ugoira

    - manga

date

  日期，不填则为最新排行。

  格式： YYYYMMDD


format

  填为 json 时会返回 json 数据。

p

  页码。

HTML 内容解析
---------------

类名为 ``ranking-items``的元素中每个 section 为一个画作。

数据使用元素上的属性存储:

id

  排行位数

data-title

  画作标题

data-user-name

  作者名称

data-date
  
  画作发布时间

data-id

  画作ID。


在 section 内部有 user-container 元素，属性包含了作者信息。

data-user_id

  作者ID。

data-user_name

  作者名称

data-profile_img

  作者头像 URL。

JSON 数据格式
----------------------


[[contents]]

title 

  画作标题

tags
 
  标签列表

url

  画作大图 URL。

illust_type

  画作类型

illust_page_count

  画作页数

user_name

  作者名称

profile_img

  作者头像

illust_id

  画作 ID。

width

  画作宽度

height

  画作高度

user_id

  作者 ID。

rank

  排名

yes_rank

  昨日排名

illust_upload_timestamp

  画作上传时间戳

