免代理直连
======================================

本页记录免代理直连相关的实测事实与机制说明。

以下结论基于 2026-09 的实测结果。网络侧封锁策略与 Pixiv 侧服务端策略都可能随时变化，
其中的数值（如配置轮换）仅为观测到的现状，不构成稳定承诺。

现状概览
----------------

按主机区分，直连能力并不一致：

.. list-table::
  :header-rows: 1

  * - 主机
    - 承载
    - 无 SNI 直连
    - ECH 直连
    - 经代理
  * - ``www.pixiv.net``
    - Cloudflare
    - 不可用
    - 可用
    - 可用
  * - ``app-api.pixiv.net``
    - Cloudflare
    - 不可用
    - 可用
    - 可用
  * - ``i.pximg.net``
    - Pixiv 自有源站
    - 可用
    - 不适用
    - 可用

``i.pximg.net`` 不在 Cloudflare 之后，其证书为 ``*.pximg.net``，因此不适用 ECH；
反之它的源站接受不携带 SNI 的握手，配合 ``Referer`` 标头即可取图。

**但取图前必须解析到真实的 pixiv 地址**，否则会表现为「无 SNI 直连不可用」。
实测同一域名在不同解析源下结果不同：

.. list-table::
  :header-rows: 1

  * - 解析源
    - 返回地址
    - 无 SNI 取图
  * - 国内 DoH（``doh.pub``、``1.12.12.12``）
    - ``23.234.30.58``
    - TCP 连接失败
  * - 境外 DoH（``dns.google``、``1.1.1.1``）
    - ``210.140.139.129``～``138``
    - 成功

对 ``210.140.139.129``～``138`` 这 10 个地址逐个验证，不发送 SNI 时全部取回原图
（``200``、``image/png``、3536464 字节），证书均为 ``pximg.net``。
因此先前若观察到该主机无 SNI 直连失败，应先检查解析结果是否落在 pixiv 网段。

SNI 阻断机制
----------------

封锁按 SNI 字段**关键词**匹配，而不是按目标 IP。因此 IP 可达不代表能建立连接：

.. list-table::
  :header-rows: 1

  * - 目标
    - SNI
    - 结果
  * - ``i.pximg.net``
    - （不发送）
    - 成功（10/10 地址）
  * - ``i.pximg.net``
    - ``i.pximg.net``
    - 连接被重置（10/10 地址）
  * - ``i.pximg.net``
    - ``pximg.net``
    - 多数成功（10 个地址中 8 个成功、2 个被重置）
  * - ``www.pixiv.net``
    - ``pixiv.net`` / ``www.pixiv.net``
    - 连接被重置

含 ``pixiv`` / ``i.pximg`` 的 SNI 会被重置，``pximg.net`` 因不含该关键词通常被放行，
但并非稳定——实测中有少数地址仍被重置，故不应依赖它作为可靠手段。
未发送 SNI 时，握手能否完成取决于服务端是否接受不携带 SNI 的连接。

不发送 SNI 的实现与失效范围
--------------------------------

本节说明 ``pkg/client/transport.go`` 中不发送 SNI 的实现，以及为何它只对 ``i.pximg.net`` 仍然有效。

机制依赖两点配合：

1. 用库自带的 DoH 解析器解析目标主机，避开被污染的系统解析
   （解析器随请求传递给自行拨号的传输，端点由 ``PIXIV_DNS_QUERY_URL`` 播种）；
2. 与源站握手时**不发送 SNI**，从而避开基于 SNI 的封锁。

第 2 步由传输原语 ``NewNoSNITransport`` 提供，以 ``TLSClientConfig`` 实现
（``ServerName`` 设为 IP 字面量，``crypto/tls`` 对 IP 字面量不生成 SNI 扩展）。
它不使用 ``DialTLSContext``：标准库文档明确后者只对 non-proxied 请求生效，
存在代理时被静默忽略。

``NewRoutedTransport`` 持有「哪些主机该用哪种方式」的清单，目前只有
``i.pximg.net`` 走不发送 SNI 的方式；``AutoTransport`` 在此之上做自动选择，
``DefaultTransport`` 默认为它。

实测表明第 2 步已被服务端拒绝：连接到源站、不发送 SNI 时，TLS 握手可以完成、
证书校验也通过（证书为 ``*.pixiv.net``），但 HTTP 层返回 nginx 的 ``403 Forbidden``。
这与社区观察一致——Pixiv 的 WAF 不再接受 SNI 与 Host 不匹配的请求。

因此该机制目前对画作 / 小说等 API 主机不再可用；对 ``i.pximg.net`` 取图仍然有效。

ECH (Encrypted Client Hello)
--------------------------------

ECH 把 ClientHello 拆成内外两层：外层使用公共的 ``cloudflare-ech.com`` 作为 SNI，
内层才是真实域名且被加密。中间设备只能看到外层名字。

要点：

- **配置由 Cloudflare 全网共享**。同一份 ECHConfig 可用于任意 Cloudflare 站点。
  实测用同一份配置访问 ``www.cloudflare.com``、``blog.cloudflare.com``、
  ``www.pixiv.net`` 均可完成握手（服务端返回的 ``ECHAccepted`` 为 true，
  说明 ECH 被真正接受而非静默回退）。
- **外层 SNI 必须是 ``cloudflare-ech.com``**。该名字是 Cloudflare 持有证书、
  用于完成外层握手的公共名，写在 ECHConfig 的 ``public_name`` 字段内。
  实测改写该字段为等长的其他名字后握手失败；使用其他组织（``defo.ie``、
  ``tls-ech.dev``）的 ECH 配置同样失败。
- **发布 ECH 配置的站点很少**。``crypto.cloudflare.com`` 与 ``cloudflare-ech.com``
  会发布；但 ``cloudflare.com``、``www.cloudflare.com``、``developers.cloudflare.com``
  等虽在 Cloudflare 之后，HTTPS 记录中并不含 ``ech=``。

获取配置
~~~~~~~~~~~~~~~~~~~~~~~~~~

有两种途径：

1. **DoH 查询 HTTPS 记录（type 65）**，从记录中取出 ``ech=`` 参数。
2. **TLS 握手**：发送一份故意的无效 ECH 配置，服务端无法解密时会在
   HelloRetryRequest 中下发正确的 ``retry_configs``。该途径不需要 DNS，
   也不需要预先持有任何配置。

DoH 端点的支持情况
++++++++++++++++++++++++++

并非所有 DoH 服务都支持查询 type 65，且**不支持时会返回空结果而非报错**，
容易与「该域名确实没有 ECH 配置」混淆。实测各主要服务的支持情况：

.. list-table::
  :header-rows: 1

  * - 支持 type 65
    - 不支持
  * - ``dns.google``
    - ``doh.pub``、``dns.alidns.com``
  * - ``1.1.1.1``
    - Quad9、AdGuard、NextDNS、Mullvad、DNS.SB、OpenDNS、ControlD

因此查询 type 65 时必须选用上表左列的端点。用支持 type 65 的端点
交叉验证 ``www.pixiv.net``、``pixiv.net``、``i.pximg.net``，
结果一致为「无 HTTPS 记录」。

构造 ECHConfigList 的要点
++++++++++++++++++++++++++

自行构造配置（例如用于自举或本地测试）时，以下几点写错会导致失败，
且部分错误在**本地就被拒绝**、请求根本发不出去：

- ``cipher_suites`` 每项是 4 字节的 ``(kdf_id, aead_id)`` 对。
  长度或取值不符时报 ``invalid cipher_suites aead_id field``。
- ``extensions`` 字段必须存在（可为空）。缺失时报 ``invalid extensions field``。
- 公钥必须是**合法的 X25519 点**。使用全零等非法值会在本地报
  ``crypto/ecdh: bad X25519 point``；若目的是让服务端无法解密，
  应使用一个合法但服务端并不持有对应私钥的公钥。
- 列表与单条的封装不同，容易写错：客户端的 ``EncryptedClientHelloConfigList``
  是**一个**位于最前的 ``uint16`` 长度（等于其余字节数），其后直接跟各条配置；
  每条配置自带 ``version+length`` 头，**条目之间不得再有长度前缀**。
  服务端的 ``EncryptedClientHelloKeys`` 则吃**裸条目**（``version+length+body``）。
  多写一层前缀会在解析时报 ``malformed ECHConfig, invalid length field``。

在库中使用 ECH
~~~~~~~~~~~~~~~~~~~~~~~~~~

``NewECHTransport`` 提供一个施加 ECH 的传输原语：接收一个底层传输作为依赖，
在其之上叠加「用加密的 ClientHello 连接」这一种能力。
它与 ``NewNoSNITransport`` 同为原语，同样不含主机判断，也不含环境判断。

.. code-block:: go

    // 直连施加 ECH：这正是 ECH 的用途，绕开按 SNI 的封锁。
    // 不要经代理跑 ECH——代理本身已经绕过了封锁，那样验证不出 ECH 是否生效。
    direct := client.New(client.WithTransport(client.NewECHTransport(&http.Transport{})))

    // 与代理组合也是支持的：底层传输带代理时，请求经代理发出且 ECH 仍然生效。
    // 但这验证的是「叠加不破坏既有管道」，不是 ECH 的作用。
    viaProxy := client.New(client.WithTransport(client.NewECHTransport(&http.Transport{
        Proxy: http.ProxyURL(proxyURL),
    })))

    // 由调用者提供配置（例如已有可靠的配置分发渠道）。
    withConfig := client.New(client.WithTransport(client.NewECHTransport(
        &http.Transport{},
        client.WithECHConfigList(myECHConfigList),
    )))

直连时若系统解析返回被污染的地址，配合本库的解析器使用（见上文「现状概览」）：

.. code-block:: go

    c := client.New(
        client.WithTransport(client.NewECHTransport(&http.Transport{})),
        client.WithDNSResolver(dns.NewDOHResolver("https://1.1.1.1/dns-query")),
    )

要点：

- **适用范围是托管在 Cloudflare 的主机**。对不在 Cloudflare 之后的主机
  （如 ``i.pximg.net``），其证书与 ECH 的外层名不匹配，ECH 不适用。
  原语不做主机分派：由了解自身环境的调用者决定何时用它。
- **外层名固定为 ``cloudflare-ech.com``**（可用 ``WithECHPublicName`` 覆盖，
  但仅当目标使用其它 ECH 提供方时才需要）。该名字写在 ECHConfig 的
  ``public_name`` 字段内，由配置携带，不由原语另行设置。
- **配置轮换自愈是运行时行为**：服务端无法解密时会下发 ``retry_configs``，
  原语用它重试并记住新配置，不需要重启或外部文件。服务端明确拒绝且未下发配置时
  返回错误，不静默退回明文握手。
- **不使用 ``DialTLSContext``**。标准库文档明确它只对 non-proxied 请求生效，
  存在代理时被静默忽略；原语用 ``TLSClientConfig`` 施加 ECH，故能与代理共存。
- **自行提供静态配置意味着承担轮换**：若配置过期而服务端又不下发新配置，
  连接会持续失败。
- 经由本库默认客户端使用时，自举同样走请求上下文中的解析器，
  因此不会被迫使用系统解析。

如何取得一份 ECHConfigList
++++++++++++++++++++++++++

一般不需要自己取：不传 ``WithECHConfigList`` 时原语会自行取得。
需要自行分发配置（例如多实例共享）时，有两种途径：

1. **DoH 查询 HTTPS 记录（type 65）**，从记录中取出 ``ech=`` 参数。
   需要选用支持 type 65 的端点，见上文「DoH 端点的支持情况」。
   注意不支持 type 65 的端点会返回空结果而非报错，容易误判为「该域名没有配置」。
2. **发送一份无法解密的配置，换取服务端下发的 ``retry_configs``**。
   这正是原语的自举途径，不需要 DNS，也不需要预先持有任何配置。

无论哪条途径，取回的配置都会轮换（见下文），因此自行分发时也必须处理更新。

轮换
~~~~~~~~~~~~~~~~~~~~~~~~~~

ECHConfig 会轮换。观测到的情况：

- DNS 记录的 TTL 在 17～300 秒之间波动；
- 连续观测中捕捉到两套不同的配置，``config_id`` 分别为 ``185`` 与 ``30``，
  两者在不同时刻交替出现。这说明轮换期间新旧配置在边缘节点上**并存**，
  同一客户端可能连续拿到其中任意一份。

因此不应把某一份 ECHConfig 视为长期有效。

社区实现的做法
++++++++++++++++++++++++++

可供参考的其他客户端做法：

- **PixEz**（Flutter）在 ECH 模式下启用 ``enableEch`` 与 ``requireEch``，
  并对图片主机单独使用不发送 SNI 的方式（``sni: false``），
  与本页「主机分两类」的结论一致。
- **pixiv-api-http**（Node）早期采用硬编码 IP 加不发送 SNI 的做法，
  其配置中的地址段现已全部不可达——硬编码 IP 会随 pixiv 调整而失效。
- **Total-ECH**（Cloudflare Workers）通过修改 DoH 返回的 HTTPS 记录，
  为解析到 CDN 的域名注入 ECH 配置，并自行处理轮换。

测试的可行性
~~~~~~~~~~~~~~~~~~~~~~~~~~

ECH 可在**完全不依赖外部网络**的条件下测试：Go 的服务端与客户端均支持 ECH，
本地起一个启用了 ECH 的服务端即可完成真实握手，并可通过连接状态中的
``ECHAccepted`` 确认 ECH 被真正接受而非静默回退。

版本要求：

- 客户端 ECH（``Config.EncryptedClientHelloConfigList``）自 Go 1.23 起提供；
- 服务端 ECH（``Config.EncryptedClientHelloKeys``）自 Go 1.24 起提供。

自愈
~~~~~~~~~~~~~~~~~~~~~~~~~~

协议本身提供了更新途径：当服务端无法解密客户端提交的 ECH 时，会在
HelloRetryRequest 中给出 ``retry_configs``。

在 Go 中，该参数通过 ``tls.ECHRejectionError`` 的 ``RetryConfigList`` 字段暴露：

.. code-block:: go

    var rej *tls.ECHRejectionError
    if errors.As(err, &rej) && len(rej.RetryConfigList) > 0 {
        // 使用 rej.RetryConfigList 重试
    }

实测流程：先用一份过期配置发起请求 → 收到附带 ``retry_configs`` 的
``ECHRejectionError`` → 改用该配置重试 → 请求成功。

需要注意，标准库**不会**自动完成这一步重试，需要调用方自行处理。
若始终使用固定的静态配置，则在配置轮换后连接会失败，直到配置被更新。

参考
----------------

- `RFC 9849: TLS Encrypted Client Hello <https://www.rfc-editor.org/rfc/rfc9849/>`_
- `Cloudflare 关于 ECH 的说明 <https://developers.cloudflare.com/ssl/edge-certificates/ech/>`_
- `Cloudflare 关于 ECH 的公告 <https://blog.cloudflare.com/encrypted-client-hello/>`_
- `net4people/bbs#417: Cloudflare ECH 的封锁特征 <https://github.com/net4people/bbs/issues/417>`_
- `XTLS/BBS#13: 通过修改 HTTPS 记录为 CDN 开启 ECH <https://github.com/XTLS/BBS/issues/13>`_
- `PixEz#1164: 直连返回 403 的讨论 <https://github.com/Notsfsssf/pixez-flutter/issues/1164>`_
- `PixEz#1253: cloudflare-ech.com 被封锁的记录 <https://github.com/Notsfsssf/pixez-flutter/issues/1253>`_
