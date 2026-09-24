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

取回图片对调用者可见的要求
--------------------------------

经代理与直连两条路径都要求请求携带 ``Referer``。实测同一地址：

.. list-table::
  :header-rows: 1

  * - 请求
    - 结果
  * - 带 ``Referer: https://www.pixiv.net/``
    - ``200``
  * - 不带 ``Referer``
    - ``403``

该要求无法从图片 URL 推知，因此由 ``Client.FetchImage`` 代为附加
（调用者已显式设置 ``Referer`` 时不覆盖）。附带说明：

- ``Content-Type`` 必须从响应读取，不能按 URL 扩展名推断：同一作品
  ``regular`` 为 ``image/jpeg``，``original`` 为 ``image/png``。
- ``Content-Length`` 并非总会返回。实测该头是否存在随网络路径与边缘节点
  变化，因此实现与调用者都不应假定它存在。
- 取回的是未经转码的原始字节，可据其校验哈希。

失败时 ``FetchImage`` 给出可行动的说明，并可用 ``errors.Is`` 分辨类别：

.. list-table::
  :header-rows: 1

  * - 错误
    - 含义
  * - ``ErrImageURLNotRecognized``
    - 入参不是可识别的 pixiv 图片或动图 zip 地址，未发出请求
  * - ``ErrImageRejected``
    - 服务端以非 200 状态码拒绝（错误文本含该状态码）
  * - ``ErrImageHostUnreachable``
    - 主机不可达或无法解析；底层原因保留在错误里

可取的地址由 ``image.IsImageURL`` 判读，按路径段识别，因此除画作各尺寸外，
小说封面、作者头像（``AuthorProfileImageURL()``）、用户背景图等
不具备各尺寸结构的图片地址，以及动图 zip 地址（``img-zip-ugoira`` 路径段，
取自 ``ugoira_meta`` 接口），同样可直接传入。该判读只看协议与路径，
不校验主机名：主机可达性由请求结果回答，且按主机校验会让本地端点
（测试用的 ``httptest`` 服务）无法被识别。

直连取图同样只需上述两点配合：先经可用途径解析到未被污染的地址，
再以不发送 SNI 的方式连接。``FetchImage`` 走客户端的传输，因此
主机与其接入方式由传输层分派，调用者不必区分尺寸或主机。

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

本节说明 ``pkg/client/transport_no_sni.go`` 中不发送 SNI 的实现，以及为何它只对 ``i.pximg.net`` 仍然有效。

机制依赖两点配合：

1. 用库自带的 DoH 解析器解析目标主机，避开被污染的系统解析
   （解析器随请求传递给自行拨号的传输，端点由 ``PIXIV_DNS_QUERY_URL`` 播种）；
2. 与源站握手时**不发送 SNI**，从而避开基于 SNI 的封锁。

第 2 步由传输原语 ``NewNoSNITransport`` 提供，以 ``TLSClientConfig`` 实现
（``ServerName`` 设为 IP 字面量，``crypto/tls`` 对 IP 字面量不生成 SNI 扩展）。
它不使用 ``DialTLSContext``：标准库文档明确后者只对 non-proxied 请求生效，
存在代理时被静默忽略。

``NewRoutedTransport`` 持有「哪些主机该用哪种方式」的清单，目前有两类：
``i.pximg.net`` 走不发送 SNI 的方式；``www.pixiv.net`` 与 ``app-api.pixiv.net``
托管在 Cloudflare，走 ECH 直连。两个通道由调用者提供，路由只负责分派，不代为
构造传输；``AutoTransport`` 用它在 ``Base`` 之上装配出默认管道，对这两类主机都会
在首选方式失败时回落到常规连接，``DefaultTransport`` 默认为它。

不发送 SNI 的握手不含主机名，标准库因此无法按主机名校验证书；而握手阶段也拿不到
目标主机名（``DialContext`` 的目标只到拨号为止，``DialTLSContext`` 虽能拿到却在经
代理时被静默忽略，故不使用）。这一步因此由路由层在收到响应后按实际协商出的证书
补齐：它知道请求主机，证书不匹配时请求报错，而不是把响应交给调用者。

因此默认配置下，对托管在 Cloudflare 的 API 主机首选 **经 ECH 直连**。``AutoTransport``
会按主机尝试若干方式（API 主机依次为 ECH、不发送 SNI、常规），定位到一个可用方式后
尽量沿用。它**不承诺任何具体的尝试顺序或记忆行为**——调用者只应依赖「请求被正确
发出，或返回错误」这一外部结果。若系统解析返回被污染的地址，连接仍会失败，需要配合
``WithDNSResolver`` 指定可用的 DoH 端点。

两种方式需要**不同的目标地址**，因此各按各自的解析目标拨号，而非在 DNS 上做一刀切的
替换：默认传输对 ``www.pixiv.net`` 的 no-SNI 腿使用带目标别名的底层
（``NewHostAliasTransport``，见 `client.NoSNIHostTarget`_）——请求 ``Host:www.pixiv.net``、
拨号解析到 ``pixiv.net`` 源站；ECH 与常规仍解析 ``www.pixiv.net``（Cloudflare）。
两者由此可以同时可用：``pixiv-doctor`` 在同一环境下实测 `API 主机 ECH 直连` 与
`API 主机无 SNI 直连` 均成功。``www.pixiv.net`` 被污染时 ECH 失败，no-SNI 仍经源站可用。

源站接受不发送 SNI 的握手、证书 ``*.pixiv.net`` 也能通过主机名校验；实测经 no-SNI
连接源站，TLS 握手与证书校验均通过、能建立 HTTP 应答（根路径返回 nginx 的
``403 Forbidden``，与 ``i.pximg.net`` 常规表现一致），但 API 路径的宽容度由服务端策略
决定，需在真实请求下验证（``pixiv-doctor`` 的 `API 主机无 SNI 直连` 探测反映连接层
可用性）。调用者**显式**提供传输时，其中的传输是明确指令，API 主机只走 ECH，不套
该 no-SNI 别名。

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

底层传输传 ``nil`` 表示由库自建，这是常规用法：自建的传输只从进程环境变量取代理，
而那不是调用者对 pixiv 的意图，故被忽略，ECH 主机直连（见下文「要点」）。
调用者若传入自己的传输，其中的代理即视为显式指定，ECH 无法直连，请求会明确报错
而不是静默降级。

.. code-block:: go

    // 直连施加 ECH：这正是 ECH 的用途，绕开按 SNI 的封锁。
    direct := client.New(client.WithTransport(client.NewECHTransport(nil)))

    // 由调用者提供配置（例如已有可靠的配置分发渠道）。
    withConfig := client.New(client.WithTransport(client.NewECHTransport(
        nil,
        client.WithECHConfigList(myECHConfigList),
    )))

直连时若系统解析返回被污染的地址，配合本库的解析器使用（见上文「现状概览」）：

.. code-block:: go

    c := client.New(
        client.WithTransport(client.NewECHTransport(nil)),
        client.WithDNSResolver(dns.NewDOHResolver("https://1.1.1.1/dns-query")),
    )

要点：

- **ECH 主机的数据连接不走代理，但按代理的来源区分**：

  - 调用者未提供传输时（默认情形），底层传输由库自建，其代理只来自进程环境
    变量（例如为了让 DoH 能出网而设的 ``HTTPS_PROXY``）。那不是调用者对 pixiv
    的意图，只是环境泄漏，故被忽略，ECH 主机直连。
  - 调用者**显式**提供带代理的传输时，那是明确的指令，库予以尊重；但经代理
    发出就依赖不了直连，ECH 无法生效，此时请求报错说明这一冲突，而不是静默
    改用普通连接（那会让调用者以为 ECH 生效了）。

  另需注意：DoH 走 ``pkg/client/dns`` 的 ``http.DefaultClient``，它自行遵循
  ``HTTPS_PROXY``，因此「给 DoH 配代理」与「给 pixiv 数据配代理」是两回事。
- **适用范围是托管在 Cloudflare 的主机**。对不在 Cloudflare 之后的主机
  （如 ``i.pximg.net``），其证书与 ECH 的外层名不匹配，ECH 不适用。
  原语不做主机分派：由了解自身环境的调用者决定何时用它。
- **同一份配置对任意 Cloudflare 主机都适用**（见上文「ECH」一节：配置由
  Cloudflare 全网共享），因此多个 Cloudflare 主机可以共用同一个原语实例，
  自举与配置轮换只发生一次，不需要按主机各建一份。
- **外层名固定为 ``cloudflare-ech.com``**（可用 ``WithECHPublicName`` 覆盖，
  但仅当目标使用其它 ECH 提供方时才需要）。该名字写在 ECHConfig 的
  ``public_name`` 字段内，由配置携带，不由原语另行设置。
- **配置轮换自愈是运行时行为**：服务端无法解密时会下发 ``retry_configs``，
  原语用它重试并记住新配置，不需要重启或外部文件。服务端明确拒绝且未下发配置时
  返回错误，不静默退回明文握手。
- **不使用 ``DialTLSContext``**。标准库文档明确它只对 non-proxied 请求生效，
  存在代理时被静默忽略；原语用 ``TLSClientConfig`` 施加 ECH。
- **自行提供静态配置意味着承担轮换**：若配置过期而服务端又不下发新配置，
  连接会持续失败。
- 经由本库默认客户端使用时，自举同样走请求上下文中的解析器。
  默认客户端不设置 ``WithDNSResolver`` 时该解析器即系统解析（见
  ``PIXIV_DNS_QUERY_URL``），ECH 主机因此可能拿到被污染的地址；
  需要自举也绕开系统解析时，用 ``client.WithDNSResolver`` 显式指定。

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
