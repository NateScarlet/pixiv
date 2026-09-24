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

  本库自行解析主机名时使用的解析方式，由取值的 URL scheme 声明。
  等价于 ``client.WithDNSResolver`` 注入一个使用该地址的解析器。

  可用写法：

  - ``http(s)://…`` —— DNS over HTTPS，查询方式由 fragment 声明（见下）。

  - ``dns://<ip>[:port]`` —— 明文 DNS，把查询发往该服务器，端口缺省 53。
    只接受 IP 字面量（IPv6 写成 ``dns://[::1]:53``）。指定主机名等于又依赖
    一次解析，而本变量的用途正是绕开系统 DNS。

  - ``dns:`` 或 ``dns://`` —— 系统解析，与 ``client.WithDNSResolver(nil)`` 同义。

  默认值为 ``dns:``（系统解析），因为公共 DoH 端点的可用性随网络环境变化，
  写死其中一个会让未设置本变量的调用者在端点不可达时整体不可用。
  需要绕开被污染的系统解析时显式设置为可用的 DoH 端点。

  DoH 默认按 `RFC 8484 <https://www.rfc-editor.org/rfc/rfc8484>`_ 的二进制报文接口查询
  （``GET`` + ``dns`` 参数携带 base64url 编码的 DNS 报文，``Accept: application/dns-message``）。
  这是标准要求实现必须支持的接口，符合标准的服务端都能用。

  若要改用 `Google 公共 DNS 的 JSON API <https://developers.google.com/speed/public-dns/docs/doh/json>`_ ，
  在网址后加上 fragment ``#type=json``（此时设置 name 参数与 ``Accept: application/dns-json``）。
  少数服务端（例如 ``dnscrypt-proxy`` 的本地 DoH 服务端）只实现二进制接口，
  用 JSON 方式查询会被以 ``400 Bad Request`` 拒绝。

  fragment 只用于向本库声明查询方式，不会发往服务端。
  声明非法（如 ``#type=binary``、``#json``）时构造客户端会 panic，
  而不是静默回落——静默回落会得到一个「端点拒绝查询」的错误，
  掩盖网址写错这个真实原因。明文 DNS 方式没有 fragment 这个概念，
  在 ``dns://`` 上写 ``#type=…`` 同样会在构造期报错。

  写法非法（缺少 scheme、未知 scheme、``dns:1.1.1.1`` 漏写 ``//``、
  ``dns://`` 后接主机名）时构造客户端会 panic，并给出正确写法。

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
