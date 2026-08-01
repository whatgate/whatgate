# WhatGate

> **普通用户推荐：桌面客户端。** 新增了 WPF/XAML 风格的跨平台客户端，支持
> Windows、macOS 和 Linux，内置首次连接引导、地区切换、启动/停止、共享出口开关和
> 运行日志。构建与安装说明见 [desktop/README.md](desktop/README.md)。

**一个邀请制的 P2P 代理分享网络。** 一群互相信任的人组成一张网，每个人既能借用别人在其他地区的出口访问受地域限制的内容，也能（在自己愿意时）成为别人的出口。没有中心服务器持有你的流量——代理数据在节点之间点对点加密直连。

> ⚠️ **成为别人的出口默认是关闭的**，必须你主动开启。开启后有一整套保护机制（信任范围、流量策略、限额熔断、本地留痕）帮你抵御滥用。请在遵守当地法律法规的前提下使用。

- **你能用它做什么**：连上一个你信任的人在目标地区的出口，让浏览器/应用的流量从那里出网。
- **为什么是 P2P**：出口是**住宅 IP**（不是机房 IP 段），更难被目标站点识别为代理；发现与准入靠邀请制，不对公众开放。
- **拿到就能跑**：编译为**单个静态二进制**，下载解压即用，不需要安装 Go 或任何运行时。

技术栈 Go + [libp2p](https://libp2p.io/)。完整架构见 [docs/architecture.md](docs/architecture.md)。

---

## 快速上手：用它上网

假设有人给了你**协调器地址**、**邀请码**，以及（推荐）协调器启动时打印的**协调器公钥**。

**1. 下载。** 到 [Releases](https://github.com/whatgate/whatgate/releases) 下对应平台的包（Windows/macOS/Linux × amd64/arm64），解压。里面的 `whatgate`（Windows 为 `whatgate.exe`）就是你要用的程序（`whatgate -version` 看版本，`SHA256SUMS` 可校验完整性）。

**2. 连上网络，按地区自动选出口。**

```bash
# -to JP：想要日本出口；-socks：本地 SOCKS5 代理监听地址
# -trust-scope：你愿意用谁的出口（conservative=只用信任圈内的；open=也用陌生人的）
whatgate -coordinator https://<host>:8080 -coordinator-key <协调器公钥> \
     -invite <邀请码> -to JP -trust-scope open -socks 127.0.0.1:1080
```

**3. 让应用走这个代理。** 把应用的 SOCKS5 代理指向 `127.0.0.1:1080`，并确保**由代理远端解析域名**（这样才能真正解锁、且不泄漏你在查哪些站点）：

```bash
# --socks5-hostname = 远端解析（正确）；不要用 --socks5（本地解析，会泄漏 DNS）
curl --socks5-hostname 127.0.0.1:1080 https://api.ipify.org   # 返回的应是出口的 IP
```

> 浏览器：Firefox 设 `network.proxy.socks_remote_dns = true`；Chrome 经 SOCKS5 时默认远端解析。DNS 解析模型与防泄漏详见 [docs/dns.md](docs/dns.md)。

### 想先试一下，手头没有协调器？

两台机器（或两个终端）直接连，不需要协调器：

```bash
whatgate -exit                                          # 终端 1：起个出口，复制它打印的某个 /p2p/ 多地址
whatgate -connect <出口多地址> -socks 127.0.0.1:1080     # 终端 2：经它建隧道
curl --socks5-hostname 127.0.0.1:1080 https://api.ipify.org
```

### 图形界面（推荐）

普通用户建议安装 `desktop/WhatGate.Desktop` 桌面客户端。连接状态、地区切换、代理地址、
共享出口、节点详情和信任圈管理都直接显示在客户端内，不需要打开 `127.0.0.1:7070`
网页，也不需要手工重启核心程序。

第一个使用者可在连接设置中直接选择“创建我的网络”，无需预先获取邀请码。客户端会
自动启动本机协调服务、登记首位管理员并生成后续成员邀请码；已有网络的成员再使用地址
和邀请码加入。

安装与打包说明见 [桌面客户端说明](desktop/README.md)。

---

## 成为出口：分享你的出口（安全第一）

加 `-exit` 就会开始为别人转发流量。**这意味着别人访问的目标会看到你的 IP**，所以出口默认带一整套保护（ExitGuard），你可以按需收紧：

```bash
# 只给自己的小网当出口、封高危端口、限并发与带宽
whatgate -coordinator https://<host>:8080 -coordinator-key <公钥> -invite <邀请码> \
     -exit -region JP \
     -group myfriends -group-secret ourSecret \
     -exit-scope conservative \
     -max-conns 50 -requester-bandwidth 5000000
```

**你能控制谁用、用来做什么：**

| 想做什么 | 用哪个 |
|---|---|
| 只服务信任圈内的人（拒陌生人） | `-exit-scope conservative` |
| 只和特定小网互信 | `-group` + `-group-secret`（首个加入者设口令，陌生人无口令进不来） |
| 拒绝低声誉/曾滥用的请求方 | `-min-reputation`（滥用者访问被封目标会被扣分，随后被各出口拒服务） |
| 封目标端口 | SMTP（25/465/587）默认封；`-block-ports` 追加 |
| 封目标域名/IP | `-block-domains`（大小写/尾点不敏感，**自动覆盖子域**，也支持 IP/CIDR） |
| 自动拉黑已知恶意域名 | `-threat-feed <url\|file>`（定期刷新） |

**防被当成免费/危险资源（默认或按需开启）：**

- **限量**：`-max-conns`（总并发）、`-max-conns-per-requester`（单人并发）、`-requester-rate`（单人建连速率，挡快开快关）、`-requester-bandwidth`（单人吞吐上限，**超了就熔断**：切断当前传输、拒其新连接、并下调其声誉，预算随时间自动回补；TCP/UDP 共用一份预算）。
- **不被拿去打内网**（SSRF 防护，**默认开启**）：出口拒绝连接私有/环回/链路本地地址（你的 LAN、`127.0.0.1`、云元数据 `169.254.169.254`）；域名按解析到的真实 IP 判定（防 DNS rebinding）。确需放开用 `-allow-private-targets`。
- **防挂死拖垮**：每条隧道默认带超时（收目标 10s、拨号 15s）。
- **可追溯**：`-audit-log <file>` 把每次服务/拒绝（时间/请求方/目标/结果）以 JSON Lines 落盘，供事后追责。
- **用可信 DNS 出网**：`-dns-server <host[:port]>` 让出口用指定解析器（隔离本地 DNS 投毒/审查），解析仍在出口侧。见 [docs/dns.md](docs/dns.md)。

---

## 自己搭一张网（运营者）

起一个协调器就有了自己的网。它只碰**元数据**（邀请准入、节点目录、中继广播），**看不到代理流量**。

```bash
coordinator -addr :8080 -invite welcome -state ./state.json -signing-key ./signing.key
```

把启动打印的**邀请码**发给你要接纳的人，把**协调器公钥**也给他们（作为 `-coordinator-key`）。出口方与客户端就能加入了（见上面两节）。

**上生产前，请逐条对照：**

- **加密控制面**：`-tls-cert`/`-tls-key`（或置于 TLS 反代后），否则邀请码/小网口令**明文过网**。
- **签名目录**：`-signing-key` 让协调器对目录与中继地址签名；节点钉住 `-coordinator-key` 后就能拒绝被 MITM 或换成恶意镜像的协调器。**强烈建议**。
- **抗刷注册**：`-rate-limit`（按 IP 限速）、`-sybil-max-identities`（同 IP 攒太多身份就隔离）。**若把协调器放在 CDN/反代后**，必须同时设 `-trusted-proxies <IP/CIDR>`，否则所有用户会被当成同一个 IP——限速互相拖累，Sybil 隔离更可能**把整个用户群锁在门外**（未配时启用会有启动告警）。
- **状态留存**：`-state` 让准入/小网/声誉跨重启保留；`-reputation-decay` 让处罚随时间淡出。
- **中继配额**：协调器兼跑 Circuit Relay v2（NAT 用户的兜底路径）；`-relay-*` 系列可限每电路时长/数据、预约与电路数，防中继被当免费带宽。

多协调器 + 客户端故障切换、断线缓存、带外自愈等抗封锁能力见下方「进阶」与 [docs/anti-censorship.md](docs/anti-censorship.md)。

---

## 进阶

<details>
<summary><b>抗封锁（协调器被封也能续命）</b></summary>

面向"协调器/中继/握手指纹被国家级防火墙盯上"的分层加固，多为默认可选：

- **响应签名 + 多端点切换 + 本地缓存**：`-coordinator` 逗号分隔多地址自动故障切换；`-coordinator-cache`（需 `-coordinator-key`）把**已验签**的目录落盘，协调器全被封时用缓存续命；一切缓存/切换都仍受签名/过期/回滚校验。
- **端口伪装**：`-listen` 加 `/ip4/0.0.0.0/tcp/443/ws` 让数据面骑 :443 像 web 流量（粗筛级，非探测抗性）。
- **带外自愈**：运营者用钉扎密钥离线签一份端点清单（`coordinator -emit-bootstrap`）托管到 CDN/GitHub raw，节点 `-bootstrap-url` 在所有已知协调器被封时拉取、验签后切换重试。

完整威胁建模与路线图见 **[docs/anti-censorship.md](docs/anti-censorship.md)**。
</details>

<details>
<summary><b>去中心化发现（🧪 实验性）</b></summary>

当协调器（含多端点/缓存/带外清单）**全部失效**时，节点还能经一张**私有认证 DHT** 发现并连上出口，不依赖任何单台服务器。信任锚是一把**离线根密钥**：它授权协调器的在线 issuer 给成员发证，出口在 DHT 上的记录都要回链到这把根才被采信；非成员记录、角色越权、被撤销者一律拒绝。

> 仅经单元测试 + 单机烟测；"断协调器仍能经 DHT 出网"需两台异网机器端到端验证。默认关闭，用 `-dht` + `-root-key` 开启。命令示例与对手驱动评审见 **[docs/c1-decentralized-discovery.md](docs/c1-decentralized-discovery.md)**。
</details>

<details>
<summary><b>全局 VPN 模式（TUN）</b></summary>

`whatgate-tun`（或 `go build -tags tun`）把**整机流量**透明导入网络，而不只是配了代理的应用。运行需管理员/root，Windows 另需 `wintun.dll`；`-tun-auto-route` 自动接管默认路由并排除自身流量。见 **[docs/tun-and-mobile.md](docs/tun-and-mobile.md)**（含移动端接入设计）。
</details>

<details>
<summary><b>可观测性（日志 / 指标）</b></summary>

- **结构化日志**：`-log-format json` 让 whatgate/coordinator 每行输出一个 JSON 对象（`level`/`msg`/字段），便于日志采集器过滤/聚合；默认 `text` 人类可读。
- **指标**：`whatgate -metrics-addr 127.0.0.1:9090` 以 JSON 暴露 `/metrics`——出口的服务量与**按原因分类的拒绝量**，用来确认限流/隔离/信任策略是否在生效。

```bash
curl -s http://127.0.0.1:9090/metrics
# { "exit_served": 128, "exit_denied:untrusted": 12, "exit_denied:requester-rate": 5 }
```

> ⚠️ `/metrics` **无鉴权**。请把 `-metrics-addr` 绑到 `127.0.0.1`（如上），或置于带鉴权的反代之后——不要绑到公网接口，否则会把出口的运营/滥用信号暴露给任何人。
</details>

<details>
<summary><b>配置文件（替代一堆命令行 flag）</b></summary>

flag 多时可写进一个 JSON 文件用 `-config` 加载——**键就是 flag 名**，命令行显式给出的覆盖文件（优先级：命令行 > 文件 > 默认）。未知键会报错（防拼写）。whatgate 与 coordinator 都支持。

```jsonc
// coord.json
{ "addr": ":8080", "invite": "welcome", "rate-limit": 5, "sybil-max-identities": 50 }
```

```bash
coordinator -config coord.json              # 全从文件读
coordinator -config coord.json -uses 7      # -uses 覆盖文件里的值
```

> ⚠️ 配置文件能设置**任意** flag，包括 `-root-key`/`-tls-key`/`-signing-key` 等密钥路径。请把它当作和命令行同等敏感——**妥善设置文件权限**，别提交进版本库。
</details>

---

## 构建与开发

从源码构建：

```bash
go build -o bin/ ./...                                # 产出 bin/coordinator、bin/whatgate
go build -tags tun -o bin/whatgate-tun ./cmd/whatgate # 可选：全局 TUN 模式
```

一键交叉编译六平台发布包到 `dist/`：`scripts/build-release.sh v0.1.0`。维护者推 `git tag v0.1.0 && git push origin v0.1.0`，GitHub Actions 会自动构建并创建 Release。

跑测试：`go test ./...`。完整测试指南（单元 + 多进程端到端出网 + 信任范围/出口保护/TUN 验证）见 **[docs/testing.md](docs/testing.md)**。

<details>
<summary><b>项目结构</b></summary>

```
cmd/coordinator   协调服务器入口（兼跑 Circuit Relay）
cmd/whatgate      节点入口（既是客户端也是出口）
internal/proxy    本地 SOCKS5 入口
internal/tunnel   隧道两端（出/入），解耦具体传输
internal/node     libp2p 接入：host、隧道、NAT 穿透、中继
internal/relay    Circuit Relay v2 中继服务
internal/coordinator  节点目录、邀请准入、信任图、HTTP 控制面
internal/trust    信任图（小网/背书/层级）、信任范围、两级声誉
internal/routing  选路引擎（地区 + 信任/延迟/负载综合排序）
internal/exit     ExitGuard 出口策略（信任范围/端口/域名/限额/熔断）
internal/tun      TUN 全局模式（tun2socks，-tags tun 构建标签）
pkg/protocol      隧道 wire 协议（目标地址编解码）
```
</details>

## 开发状态

核心链路（M1–M6）已完成：本地 SOCKS5 + libp2p 直连出网、邀请准入 + 目录发现 + NAT 穿透 + 中继兜底、小网/信任/声誉、地区+延迟+负载综合选路、ExitGuard 出口治理、TUN 全局模式（代码完成待真机验证）。

后续增强、抗封锁路线图与真机验证清单见 **[docs/backlog.md](docs/backlog.md)** 与 **[docs/anti-censorship.md](docs/anti-censorship.md)**。

> 真实跨 NAT 打洞与 AutoRelay 自动预约需在两台异网机器上实测；本地回环已验证隧道与中继数据路径本身。

## 许可

[MIT](LICENSE) © 2026 WhatGate
