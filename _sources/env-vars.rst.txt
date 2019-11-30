环境变量
=========================

本库的默认行为受到环境变量的控制。

PIXIV_PHPSESSID

  默认客户端的 PIXIV 会话 Cookie 值，用于绕过 reCAPTCHA 登录验证。

PIXIV_BYPASS_SNI_BLOCKING

  设为非空来启用默认客户端的免代理功能

PIXIV_DNS_QUERY_URL

  免代理所需要的 DNS over HTTPS 使用的服务网址。
  
  服务器接口应类似于 `Google 公共 DNS 的 JSON API <https://developers.google.com/speed/public-dns/docs/doh/json>`_ ，大部分公共 DNS 服务应该都支持这个格式。
  
  使用时将设置 name 参数和 Accept:application/dns-json。

  部分可用 DoH 服务网址：

  - ``https://1.0.0.1/dns-query``

  -	``https://1.1.1.1/dns-query``

  - ``https://cloudflare-dns.com/dns-query``

  - ``https://dns.nextdns.io/dns-query``
