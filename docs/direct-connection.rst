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
    - 成功
  * - ``i.pximg.net``
    - ``i.pximg.net``
    - 连接被重置
  * - ``i.pximg.net``
    - ``pximg.net``
    - 成功
  * - ``www.pixiv.net``
    - ``pixiv.net`` / ``www.pixiv.net``
    - 连接被重置

含 ``pixiv`` / ``i.pximg`` 的 SNI 会被重置，``pximg.net`` 因不含该关键词而放行。
未发送 SNI 时，握手能否完成取决于服务端是否接受不携带 SNI 的连接。

``BypassSNIBlocking`` 的失效
--------------------------------

本节说明 ``pkg/client/bypass_sni_blocking.go`` 现有机制为何不再能完成直连。

机制原本依赖两点配合：

1. ``www.pixiv.net`` 解析到 Cloudflare 地址，而无 SNI 时 Cloudflare 无法路由，
   因此通过 ``HostReplacer`` 把 ``www.pixiv.net`` 改写为 ``pixiv.net`` 再解析，
   以获得 Pixiv 自有源站地址；
2. 与源站握手时**不发送 SNI**，从而避开基于 SNI 的封锁。

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
   注意并非所有 DoH 服务都支持查询 type 65。
2. **TLS 握手**：发送一份故意的无效 ECH 配置，服务端无法解密时会在
   HelloRetryRequest 中下发正确的 ``retry_configs``。该途径不需要 DNS，
   也不需要预先持有任何配置。

轮换
~~~~~~~~~~~~~~~~~~~~~~~~~~

ECHConfig 会轮换。观测到的情况：

- DNS 记录的 TTL 在 17～300 秒之间波动；
- 连续观测中捕捉到两套不同的配置，``config_id`` 分别为 ``185`` 与 ``30``，
  两者在不同时刻交替出现。这说明轮换期间新旧配置在边缘节点上**并存**，
  同一客户端可能连续拿到其中任意一份。

因此不应把某一份 ECHConfig 视为长期有效。

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
