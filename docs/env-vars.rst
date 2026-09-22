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

  默认按 `RFC 8484 <https://www.rfc-editor.org/rfc/rfc8484>`_ 的二进制报文接口查询
  （``GET`` + ``dns`` 参数携带 base64url 编码的 DNS 报文，``Accept: application/dns-message``）。
  这是标准要求实现必须支持的接口，符合标准的服务端都能用。

  若要改用 `Google 公共 DNS 的 JSON API <https://developers.google.com/speed/public-dns/docs/doh/json>`_ ，
  在网址后加上 fragment ``#type=json``（此时设置 name 参数与 ``Accept: application/dns-json``）。
  少数服务端（例如 ``dnscrypt-proxy`` 的本地 DoH 服务端）只实现二进制接口，
  用 JSON 方式查询会被以 ``400 Bad Request`` 拒绝。

  fragment 只用于向本库声明查询方式，不会发往服务端。
  声明非法（如 ``#type=binary``、``#json``）时构造客户端会 panic，
  而不是静默回落——静默回落会得到一个「端点拒绝查询」的错误，
  掩盖网址写错这个真实原因。

  部分可用 DoH 服务网址：

  - ``https://1.0.0.1/dns-query``

  -	``https://1.1.1.1/dns-query``

  - ``https://cloudflare-dns.com/dns-query``

  - ``https://dns.nextdns.io/dns-query``

  自建的 DoH 端点照常按系统信任根校验证书，无需额外配置：
  只要其 CA 已装入系统信任根即可（Windows 上即「受信任的根证书颁发机构」）。

  若端点只支持 JSON 接口，写成 ``https://doh.example.com/dns-query#type=json``。

已移除的环境变量
--------------------

PIXIV_BYPASS_SNI_BLOCKING

  原先用于为默认客户端启用免代理。现在「哪个主机需要哪种接入方式」由库内的路由传输决定
  （``client.DefaultTransport`` 默认为 ``client.AutoTransport``），不再需要调用者开关。
  需要完全自行控制时，用 ``client.WithTransport`` 注入自己组合的传输。
