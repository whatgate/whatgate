# WhatGate 安全评审

> 首次评审日期：2026-07-25 · 范围：签名鉴权、协调器 API、邀请/准入、ExitGuard、隧道/UDP 数据面、wire 框架。
> 状态标记：`open` 待修 · `in-progress` 修复中 · `fixed` 已修复（附提交）。

## 结论概览

底层身份/加密/框架设计扎实——libp2p 提供认证加密的数据面，签名鉴权正确绑定
`公钥→PeerID→动作→时间戳`，长度前缀框架均有界。问题集中在**出口治理的策略深度**
与**协调面的运维加固**。最严重一条是经典的代理 SSRF（F1）。

建议修复顺序：**F1 → F3 → F4**（出口侧，护普通用户的机器与内网）→ **F2**（信任系统抗操纵）
→ **F5/F6/F7**（协调面加固）→ **F8**（凭据卫生）。

| 编号 | Issue | 严重度 | 概述 | 状态 |
|---|---|---|---|---|
| F1 | [#1](https://github.com/whatgate/whatgate/issues/1) | 🔴 HIGH | 出口 SSRF：可连内网/本机/云元数据 | ✅ fixed |
| F2 | [#2](https://github.com/whatgate/whatgate/issues/2) | 🔴 HIGH | 声誉举报可被操纵，架空 `-min-reputation` | open |
| F3 | [#3](https://github.com/whatgate/whatgate/issues/3) | 🟠 MEDIUM | 域名黑名单精确串匹配，易绕过；不挡 IP | ✅ fixed |
| F4 | [#4](https://github.com/whatgate/whatgate/issues/4) | 🟠 MEDIUM | 出口侧无超时，slowloris/挂死可拖垮 | open |
| F5 | [#5](https://github.com/whatgate/whatgate/issues/5) | 🟠 MEDIUM | 协调面明文 HTTP，泄露邀请码/小网口令 | open |
| F6 | [#6](https://github.com/whatgate/whatgate/issues/6) | 🟡 LOW-MED | 协调器 JSON 端点无请求体大小限制 | open |
| F7 | [#7](https://github.com/whatgate/whatgate/issues/7) | 🟡 LOW | 2 分钟重放窗口，无 nonce | open |
| F8 | [#8](https://github.com/whatgate/whatgate/issues/8) | 🟡 LOW | 远程 URL 内嵌明文 PAT（凭据卫生） | open |

---

## 🔴 F1 — 出口 SSRF：可被用来打穿出口的内网 / 本机 / 云元数据（HIGH）

**位置**：`cmd/node/main.go`（`d.DialContext(ctx,"tcp",addr)`）、`internal/node/udp.go`
（`ResolveUDPAddr`+`DialUDP`）、`internal/exit/guard.go`（`Authorize` 不校验目标 IP 段）。

`ExitGuard` 只按端口/域名字符串/信任范围/声誉/并发过滤，**完全不限制目标 IP 段**。
任何在出口信任范围内的请求方（`-exit-scope open` 时即任何人）都能让出口去连：

- `169.254.169.254:80` → **云厂商元数据端点，偷 IAM 凭证/临时密钥**（出口跑 VPS 时致命）
- `127.0.0.1:*` / `[::1]` → 出口本机管理服务、其它本地端口（含出口自己的 `-web` 控制台、SOCKS 口）
- `192.168.x.x` / `10.x` / `172.16.x` → 出口所在 LAN 的路由器后台、NAS、内网服务

对一个"借用住宅 IP"的网络，这是最高危面：出口方把**自己的内网和本机**暴露给了陌生请求方。

**修复**：在 `Guard.Authorize` 加目标校验——若 Host 是 IP 字面量，拒绝私有/环回/链路本地/
`169.254.169.254`/ULA；若是域名，**在出口 Dial 后校验解析到的 IP**（防 DNS rebinding / 指向内网的域名）。
加 `Policy.AllowPrivateTargets bool`（默认 false）。UDP 侧 `relayUDP` 同样把关。

> ✅ **已修复**（commit 见 `Fixes #1`）：新增 `exit.DisallowedTargetIP`（环回/私有/链路本地/ULA/
> unspecified/multicast）+ `exit.DialControl`（net.Dialer.Control 拨号时按解析 IP 拒，含 rebinding）；
> `Policy.AllowPrivateTargets`（默认 false）；`Guard.Authorize` 拒 IP 字面量私网目标；UDP `relayUDP`
> 解析后按真实 IP 拒；cmd `-allow-private-targets`。测试：`DisallowedTargetIP` 表驱动、guard IP 字面量拒/
> 公网放/opt-in 放行、UDP 环回目标被丢。

---

## 🔴 F2 — 声誉举报可被操纵，直接架空 `-min-reputation` 门槛（HIGH）

**位置**：`internal/coordinator/api.go` `handleReport`。

任何**已准入**成员都能对**任意 Subject**上报结果，唯一校验是"举报者已准入 + 签名有效"。
没有"举报者确实服务过该 Subject"的绑定，无去重/限频：

- **打压**：反复 POST `{subject: 受害者, outcome: blocked}` → 把任意节点声誉刷到极低 →
  被所有开了 `-min-reputation` 的出口拒服务；连带 `AdjustGroup` 拖垮其整组声誉。
- **养号**：对**自己**反复 `served`（+1）刷高声誉，绕过门槛。
- **重放**：同一条签名举报在 2 分钟窗口内可重复提交，delta 被重复计入。

**修复**：把举报与一次真实出口会话绑定（出口对会话签发短票据，举报须附）；至少加
(reporter, subject) 去重 + 限频 + 单方影响封顶；`served` 不应由被服务方自证。

---

## 🟠 F3 — 域名黑名单形同虚设，且只挡域名不挡 IP（MEDIUM）

**位置**：`internal/exit/guard.go` `isDomainBlocked` = `blockedDomains[host]` 精确串匹配。

封了 `evil.example.com`，以下全部绕过：`EVIL.example.com`（大小写）、`evil.example.com.`（尾点）、
`sub.evil.example.com`（子域）、或直接用其解析后的 IP 字面量。威胁情报 feed 也只喂域名。

**修复**：匹配前 `ToLower` + 去尾点；按域名**后缀**匹配（含子域）；封锁集支持 IP/CIDR，
并在 Dial 解析出 IP 后再查一次（与 F1 合并实现）。

> ✅ **已修复**（`Fixes #3`）：`normalizeHost`（lower+去尾点）；`isHostBlocked` 逐级父域后缀匹配
> （封 `evil.example` 覆盖 `sub.evil.example`，不误伤 `notevil.example`）；`parseBlockSet` 把封锁集
> 分成域名 + IP/CIDR 两组，IP 字面量目标按 CIDR 命中。`SetBlockedDomains`（威胁情报刷新）同样重解析。
> 测试覆盖大小写/尾点/子域/非子域、IP 与 CIDR 命中。（说明：域名→内网 IP 的 rebinding 已由 F1 的
> `DialControl` 兜住；把 threat-feed 的 CIDR 也在 Dial 后套用属后续增强。）

---

## 🟠 F4 — 出口侧无超时，可被 slowloris / 挂死拖垮（MEDIUM）

**位置**：`internal/tunnel/tunnel.go`。

`ReadTarget` 用 `io.ReadFull` **无读超时**：请求方开流、只发半个长度前缀就不动，出口 goroutine +
声誉/并发槽（`exitLoad++`、Guard 的 `active`）被无限期占住。`dial(context.Background(), addr)`
**无拨号超时**；`pipe` **无空闲超时**。配合无"每请求方连接上限"，单 peer 即可耗尽出口。

**修复**：读 target 用 `SetReadDeadline`（数秒）；`dial` 用 `context.WithTimeout`；`pipe` 加空闲超时；
`Policy` 加每-requester 并发上限。

---

## 🟠 F5 — 协调面明文 HTTP，泄露邀请码与小网口令（MEDIUM）

**位置**：部署用 `http://`；`joinRequest.Code`、`joinGroupRequest.Secret` 明文传输。

签名鉴权防冒名，但防不了窃听。在途攻击者能抓到：**小网口令**（进信任圈的钥匙）、
**邀请码**（复用未耗尽的可自行入网）、完整在线目录。

**修复**：协调器上 TLS（文档强制 https + 证书校验）；口令类字段改为"证明知道口令"
（HMAC 挑战）而非明文传口令。

---

## 🟡 F6 — 协调器 JSON 端点无请求体大小限制（LOW–MEDIUM）

`json.NewDecoder(r.Body).Decode(...)` 无 `http.MaxBytesReader`；`register` 的 `Addrs []string`
无上限并存进目录再广播。恶意已准入节点可发超大 body（内存 DoS）或塞巨量 Addrs（放大）。

**修复**：所有 handler 包 `http.MaxBytesReader(w, r.Body, 64<<10)`；限制 `Addrs` 条数/长度；
给 HTTP server 设 `ReadHeaderTimeout`。

---

## 🟡 F7 — 2 分钟重放窗口、无 nonce（LOW）

签名请求在 `authMaxSkew=2min` 内可原样重放；`/report` 的 delta 会被重复计入（见 F2）。

**修复**：协调器记最近用过的签名（`sig` 去重）或引入一次性 nonce。

---

## 🟡 F8 — 远程 URL 内嵌明文 PAT（凭据卫生，LOW）

`git remote` 的 origin URL 内嵌了明文 GitHub PAT，存于 `.git/config`，会经 `git remote -v`、
屏幕共享、备份泄露。

**修复**：吊销该 token；`git remote set-url origin https://github.com/whatgate/whatgate.git`，
改用 Git Credential Manager 保存凭据，不入 URL。

---

## ✅ 做得对的地方（回归时勿破坏）

- 签名规范串含 `action`，杜绝跨动作重放；`checkAuth` 强制 `auth.PeerID==claimedPeerID`；
  `group/join` 绑定 `groupID+peerID`、`endorse` 校验成员资格 —— 鉴权链正确。
- 框架长度前缀有界（target `uint16`、payload ≤ 65535），无未绑定分配。
- `Guard` 的 release 用 `sync.Once`，无重复释放并发槽。
- 数据面走 libp2p 认证加密，业务流量不过协调器。
