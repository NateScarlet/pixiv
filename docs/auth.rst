授权
----------------

Pixiv 被墙所以需要配置代理。

不登录时无法看到 R-18 内容，登录需要 Cookie 值 ``PHPSESSID``。

同时P 站现在会屏蔽特定User-Agent，需要手动指定未被屏蔽的 User-Agent。

图片带有 referer 标头检查，需要本地缓存。




判断是否登录
~~~~~~~~~~~~~~~~~~~~~~~~~~

访问 ``HEAD https://www.pixiv.net/setting_user.php``

状态码 200 为登录 302 为未登录， 其他状态码报错。

直接设置 Cookie
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

R-18 内容需要登录。直接设置 ``PHPSESSID`` 即可，避免走已不支持的账号密码登录。

用 ``client.WithPHPSESSID`` 选项提供，例如 ``client.New(client.WithPHPSESSID(os.Getenv("PIXIV_PHPSESSID")))``。

未显式设置时，如果存在 ``PIXIV_PHPSESSID`` 变量将尝试直接使用此值作为登录凭据。


