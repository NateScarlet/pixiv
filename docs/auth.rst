授权
----------------

Pixiv 被墙所以需要配置代理。

不登录时无法看到 R-18 内容，登录需要 Cookie 值 ``PHPSESSID``。

图片带有 referer 标头检查，需要本地缓存。


登录
====================

登录表单提交地址 ``https://accounts.pixiv.net/api/login?lang=zh``

表单数据:

pixiv_id

    用户名

password

    密码

post_key

    登录页面 ``https://accounts.pixiv.net/login?lang=zh`` 上 ``input[name="post_key"]`` 元素的 ``value`` 属性。

提交表单时请求必须带 ``post_key`` 对应的 ``PHPSESSID`` Cookie。

成功时返回值:

.. code-block:: json

    {
        "body": {
            "success": {
                "return_to": "https://www.pixiv.net/"
            }
        },
        "error": false,
        "message": ""
    }

登录时会读取 ``PIXIV_USERNAME`` 和 ``PIXIV_PASSWORD`` 环境变量，按需要进行登录。

判断是否登录
~~~~~~~~~~~~~~~~~~~~~~~~~~

访问 ``HEAD https://www.pixiv.net/setting_user.php``

状态码 200 为登录 302 为未登录， 其他状态码报错。

直接设置 Cookie
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

自动登录有时会触发 reCAPTCHA 验证， 所以支持直接设置 ``PHPSESSID``。

如果存在 ``PIXIV_PHPSESSID`` 变量将尝试直接使用此值作为登录凭据，登录无效时再尝试使用账号密码登录。


