环境变量
=========================

本库的默认行为受到环境变量的控制。

环境变量是**默认值的播种来源**：只有调用 ``client.New`` 时没有用对应选项显式设置的项才会采用环境变量的值。
想为单个客户端指定不同的值，用选项而不是环境变量。

PIXIV_PHPSESSID

  默认客户端的 PIXIV 会话 Cookie 值，用于绕过 reCAPTCHA 登录验证。
  等价于 ``client.WithPHPSESSID``。

PIXIV_USER_AGENT

  对 Pixiv 请求使用的 User-Agent 标头，默认使用 ``Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:84.0) Gecko/20100101 Firefox/84.0``。
  等价于 ``client.WithUserAgent``。

PIXIV_DNS_QUERY_URL

  免代理所需要的 DNS over HTTPS 使用的服务网址。
  等价于 ``client.WithDNSResolver`` 注入一个使用该地址的解析器。

  服务器接口应类似于 `Google 公共 DNS 的 JSON API <https://developers.google.com/speed/public-dns/docs/doh/json>`_ ，大部分公共 DNS 服务应该都支持这个格式。

  使用时将设置 name 参数和 Accept:application/dns-json。

  部分可用 DoH 服务网址：

  - ``https://1.0.0.1/dns-query``

  -	``https://1.1.1.1/dns-query``

  - ``https://cloudflare-dns.com/dns-query``

  - ``https://dns.nextdns.io/dns-query``

已移除的环境变量
--------------------

PIXIV_BYPASS_SNI_BLOCKING

  原先用于为默认客户端启用免代理。现在「哪个主机需要哪种接入方式」由库内的路由传输决定
  （``client.DefaultTransport`` 默认为 ``client.AutoTransport``），不再需要调用者开关。
  需要完全自行控制时，用 ``client.WithTransport`` 注入自己组合的传输。
