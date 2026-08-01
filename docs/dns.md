# DNS 策略与防泄漏

代理的一个核心承诺：让**目标域名在出口侧解析**，而不是在你（客户端）本地解析。这有两层意义：

- **解锁**：geo 限制的站点常按 DNS 解析位置返回不同结果。在出口所在国解析，才能拿到当地视角的地址。
- **防泄漏**：若域名在你本地解析，你的 ISP/防火墙就能看到你要访问哪些站点（即使随后的连接走了代理），并可对 DNS 投毒/过滤。让出口解析可避免这条泄漏与投毒面。

## 解析发生在哪

WhatGate 隧道端到端传的是 **`host:port` 目标**，出口收到后才 `dial`（触发解析）。所以：

| 客户端如何交目标 | 解析位置 | 是否泄漏 |
|---|---|---|
| 发**主机名**（SOCKS5 `atypDomain`）| **出口侧** ✅ | 否 ✅ |
| 发**已解析的 IP** | 客户端本地已解析 | 是（本地 DNS 已暴露）|

WhatGate 的 SOCKS5 入口对 `atypDomain` **原样透传主机名**（不在本地解析），因此只要你的客户端把主机名交给代理，就走出口解析。

## 客户端怎么配（关键）

leak 与否取决于**你的客户端**是否把主机名交给代理：

- **curl**：用 `--socks5-hostname 127.0.0.1:1080`（远端解析）——**不要**用 `--socks5`（本地解析，会泄漏）。
- **Firefox**：`network.proxy.socks_remote_dns = true`。
- **Chrome/Chromium**：经 SOCKS5 代理时默认远端解析。
- **应用/库**：确认其 SOCKS5 走 "remote/hostname" 模式而非先本地解析再连 IP。

验证不泄漏：

```bash
curl --socks5-hostname 127.0.0.1:1080 https://api.ipify.org   # 返回出口 IP
```

## 出口侧：可配解析器（`-dns-server`）

出口默认用**其主机的系统解析器**解析主机名目标。若出口所在网络的 ISP 解析器被投毒/过滤，可用 `-dns-server` 钉一个可信解析器：

```bash
whatgate -exit -region JP -dns-server 1.1.1.1        # 或 8.8.8.8、host:port（默认 :53）
```

解析**仍在出口侧**发生，只是改由指定服务器应答，隔离本地 DNS 的投毒/审查面。空值 = 系统解析器。

## 诚实边界

- **UDP 主机名目标**：UDP 出口目前用系统解析器解析主机名（`-dns-server` 只覆盖 TCP 拨号）。注意 DNS-over-UDP 本身通常把目标写成 **IP 字面量**（如 `1.1.1.1:53`），此时无需出口解析，不受影响。
- **SSRF 防护叠加**：无论用哪个解析器，出口都会在拨号时按**解析到的真实 IP** 拒绝私有/环回/链路本地目标（防 DNS rebinding），见 [security-review.md](security-review.md) 与 `-allow-private-targets`。
- **TUN 全局模式**：全局模式下的 DNS 分流规则见 [tun-and-mobile.md](tun-and-mobile.md)（IPv6/分流为后续项）。

## 相关

- ExitGuard 与 SSRF：README「出口保护」段。
- 结构化日志：出口钉定解析器时会记 `exit DNS resolver pinned`（`-log-format json` 可采集）。
