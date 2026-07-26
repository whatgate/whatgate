# WhatGate

一个**邀请制的 P2P 代理分享网络**：所有在线用户组成一张网，每个节点**既是客户端也是出口**。用户可以在网内按地区寻找最合适的出口来访问受地域限制的内容，同时（在知情自愿的前提下）也充当别人的出口。

> ⚠️ **安全与合规底线**：成为别人的出口**默认关闭**，必须用户显式开启。出口方拥有信任范围限定、流量策略、限额熔断与本地留痕等保护机制，用于抵御被非法之徒滥用的风险。请在遵守当地法律法规的前提下使用。

架构细节见 [docs/architecture.md](docs/architecture.md)。

## 核心设计

- **场景**：地理位置解锁（借用目标地区节点的出口 IP）。
- **信任结构**：分层联邦——全局「大网」之下保留「小网」（Group），小网内部与小网之间可互相邀请、认证、评声誉。
- **两个平面**：
  - **控制面**（Coordinator，HTTP）：只碰元数据——邀请准入、节点目录、中继广播。看不到业务流量。
  - **数据面**（libp2p）：代理流量**点对点加密直连**；打洞失败时走 Circuit Relay v2 中继兜底。
- **协议**：TCP（SOCKS5 CONNECT）与 UDP（SOCKS5 UDP ASSOCIATE，覆盖 DNS/游戏/音视频）均经加密隧道转发。
- **技术栈**：Go + [libp2p](https://libp2p.io/)。
- **分发**：编译为单个静态二进制，终端用户下载即用，无需安装任何运行时。

## 快速开始

### 下载预编译二进制（推荐）

到 [**Releases**](https://github.com/whatgate/whatgate/releases) 下载对应平台的压缩包（Windows/macOS/Linux，amd64 与 arm64），解压即用——**无需安装 Go 或任何运行时**。每个包内含：

- `coordinator` — 协调服务器
- `node` — 参与节点（精简版：SOCKS5 代理）
- `node-tun` — 参与节点（全局 TUN/VPN 模式；运行需管理员/root，Windows 另需 `wintun.dll`）

用 `SHA256SUMS` 可校验完整性；`node -version` / `coordinator -version` 打印版本。

> 维护者发布新版本：`git tag v0.1.0 && git push origin v0.1.0`，GitHub Actions（[`.github/workflows/release.yml`](.github/workflows/release.yml)）会自动交叉编译六平台并创建 Release。

### 从源码构建

```bash
go build -o bin/ ./...
```

产出 `bin/coordinator` 与 `bin/node`（Windows 为 `.exe`）。

全局 TUN 模式（可选，体积更大、需管理员权限运行）：

```bash
go build -tags tun -o bin/node-tun ./cmd/node
```

（运行方式与平台前置见 [docs/tun-and-mobile.md](docs/tun-and-mobile.md)。）

一键交叉编译全平台发布包（产出到 `dist/`）：

```bash
scripts/build-release.sh v0.1.0
```

### 方式 A：手动直连（无需协调器）

```bash
# 终端 1：起一个出口节点，复制它打印的某个 /p2p/ 多地址
node -exit

# 终端 2：起客户端，通过该出口建隧道
node -connect <出口多地址> -socks 127.0.0.1:1080

# 验证流量确实从出口出网（返回的应是出口的公网 IP）
curl --socks5-hostname 127.0.0.1:1080 https://api.ipify.org
```

### 方式 B：经协调器发现（邀请准入 + 按地区选出口）

```bash
# 协调器（兼跑中继），播种邀请码；-state 让准入/小网/声誉跨重启留存
# 声誉每小时向 0 衰减（-reputation-decay），被罚节点自动恢复
# 生产务必启用 TLS（-tls-cert/-tls-key，或置于 TLS 反代后），否则邀请码/小网口令明文过网
# -signing-key 让协调器签名目录响应，启动会打印“协调器公钥”，把它作为节点的 -coordinator-key
coordinator -addr :8080 -invite welcome -state ./coordinator-state.json -signing-key ./coordinator-signing.key

# 出口节点：加入并注册为某地区(如 JP)出口（-coordinator-key 钉住协调器公钥）
node -coordinator https://<host>:8080 -coordinator-key <协调器公钥> -invite welcome -exit -region JP

# 客户端：加入并按地区自动发现出口（多协调器逗号分隔可故障切换；-coordinator-cache 缓存目录抗断线）
node -coordinator https://a:8080,https://b:8080 -coordinator-key <协调器公钥> -coordinator-cache ./dir.cache -invite welcome -to JP -socks 127.0.0.1:1080

curl --socks5-hostname 127.0.0.1:1080 https://api.ipify.org
```

> 🛡️ **抗封锁（A0/A1/A3/A4/C2；C1 见下）**：
> - **响应认证（A0）**：协调器 `-signing-key` 对**目录响应与中继地址**签名；节点 `-coordinator-key` 钉住其公钥后，**校验签名并拒绝**未签名/别的密钥签名/被回滚（旧序号）的目录与中继——协调器被 MITM 或换成恶意镜像也无法把你导向敌手出口或对手控制的中继。未钉公钥会告警。
> - **多端点故障切换（A1）**：`-coordinator` 逗号分隔多个地址；某个被封（连不上）自动切下一个，"业务错误"（能连但拒）则如实上抛不误切。为真正独立，多个端点应跨不同故障域（ASN/DNS 商/CDN/运营者）。
> - **本地目录缓存（A3）**：`-coordinator-cache`（需 `-coordinator-key`）把**已验签**的目录落盘（仅本人可读）；协调器全被封时用缓存续命，避免"一封就断"。缓存仍受签名/过期/回滚校验，过期即拒；用缓存时会打印告警。
> - **TLS（`https://`）** 另外保护控制面**机密性**（问了哪些出口不外泄），与签名互补。
>
> - **端口伪装（A4）**：`-listen` 支持逗号分隔多地址；加 `/ip4/0.0.0.0/tcp/443/ws` 让数据面骑 **:443** 像 web 流量。这只是**粗筛级伪装**（非探测抗性）；要真像 HTTPS 需 `/tls/ws`+证书或 TLS 反代，多数住宅节点无公网入站则保持拨号-only 经中继。
> - **带外引导自愈（C2）**：运营者 `coordinator -emit-bootstrap "https://a:8080,https://b:8080" -signing-key ...` 用钉扎密钥离线签一份端点清单（打印 JSON），托管到 CDN/GitHub raw 等**难封渠道**；节点 `-bootstrap-url <该地址>`（需 `-coordinator-key`）在**所有已知协调器都被封**时拉取该清单、**验签通过后切到新端点重试**——冷启动 join 与运行中目录获取**都会自愈**（返场节点亦然），协调器整批被封也能自我修复。篡改的 CDN 无法投毒（信任只来自签名）；未钉公钥则拒绝（不认证清单本身是投毒面）。
>
> 整体设计与分阶段路线图（含 Tier B 探测抗性、Tier C 去中心化发现）见 **[docs/anti-censorship.md](docs/anti-censorship.md)**。

### 去中心化发现（Tier C1，🧪 实验性）

当协调器（及其多端点/缓存/带外清单）**全部失效**时，节点还能经一张**私有认证 DHT**发现并连上出口——发现不再依赖任何单台服务器。信任锚是一把**离线根密钥**：它签发协调器用的"在线受限 issuer"，issuer 在准入时给成员发**成员证书**，出口在 DHT 上广告的记录都要回链到这把根才被采信。

> ⚠️ **实验性**：`-dht` 路径仅经编译 + 单元测试 + 本地单机烟测验证；**门控/中继/DHT/NAT 打洞的真实交互、以及"断协调器仍能经 DHT 出网"需两台异网机器端到端验证**（与 TUN/跨 NAT 一致）。默认关闭，不影响常规使用。

```bash
# 1) 离线机（保管根密钥）：为协调器的在线 issuer 公钥签一张授权 {member} 的 issuer 证书。
#    先在协调器机上 `coordinator -issuer-key ./issuer.key`（会打印 issuer 公钥），把它填到 -issuer-pubkey；
#    stderr 打印的“根公钥”就是节点要钉的 -root-key。
coordinator -emit-issuer-cert -root-key ./root.key \
  -issuer-pubkey <issuer公钥> -issuer-roles member -issuer-id coord-issuer > issuer-cert.json

# 2) 协调器（在线）：带上 issuer 私钥 + 上一步的根签证书；此后每次 join 都给成员发一张 {member} 证书。
coordinator -addr :8080 -invite welcome -signing-key ./coordinator-signing.key \
  -issuer-key ./issuer.key -issuer-cert ./issuer-cert.json

# 3) 节点：钉住根公钥、开启 DHT。join 时领到成员证书（-member-cert 落盘）；
#    协调器可达时走目录，全被封时也能经私有 DHT 发现已验证出口。
node -coordinator https://<host>:8080 -coordinator-key <协调器公钥> -invite welcome \
  -dht -root-key <根公钥> -member-cert ./member.cred -to JP -socks 127.0.0.1:1080

# 撤销：离线根签一份撤销 checkpoint（单调版本、带陈旧上限），经难封渠道分发。
coordinator -emit-revocation -root-key ./root.key \
  -revoke-subjects <被撤销PeerID> -revocation-version 2 > revocation.json
```

- **出口角色**须离线根/双签授权（在线 issuer 只能签 `{member}`，签不出 `{exit}`）：给某 issuer 加 `-issuer-roles member,exit` 才能签发出口证书，再据此让出口在 DHT 上广告。
- 私有 DHT 上**非成员的记录一律不采信**（回链根验签 + 角色核对 + 反回滚 + SSRF 地址过滤 + equivocation 隔离）；DHT 发现的出口标为"陌生"，**保守信任范围下自动排除**（DHT 不提供"推荐"，那只来自协调器的权威声誉）。

设计与对手驱动评审见 **[docs/c1-decentralized-discovery.md](docs/c1-decentralized-discovery.md)**。

### 本地状态面板

任意节点加 `-web` 即可在浏览器查看实时状态并**运行时操作**：切换出口地区、一键开关"作为出口"（均无需重启）。客户端若没给 `-trust-scope`，首次打开面板会弹出**信任范围向导**（解释风险后让你选保守/开放）再连接出口。面板还能**管理小网**（加入/创建、给小网背书、查看我的小网）：

```bash
node -coordinator http://host:8080 -invite welcome -to JP -trust-scope open -web 127.0.0.1:7070
# 浏览器打开 http://127.0.0.1:7070
```

### 出口保护（ExitGuard）

作为出口时可加保护策略，抵御被陌生人滥用：

```bash
# 只给自己小网/已认证小网当出口；封禁高危端口；限并发
node -coordinator http://host:8080 -invite welcome -exit -region JP \
     -group myfriends -group-secret ourSecret \
     -exit-scope conservative \
     -block-domains evil.example.com \
     -max-conns 50
```

- `-group` + `-group-secret`：加入小网（首个加入者设口令；成员用同一口令自证入组，陌生人无口令进不来）
- `-exit-scope conservative`：只服务信任圈内的请求者（陌生人被拒）
- SMTP 端口（25/465/587）默认封禁；`-block-ports` 追加
- `-block-domains`：目标黑名单——大小写/尾点不敏感，**自动覆盖子域**（封 `evil.com` 也封 `x.evil.com`），条目也可写 IP 或 CIDR（如 `203.0.113.0/24`）
- `-max-conns`：最大并发连接数；`-max-conns-per-requester`：单个请求方的并发上限（防单点耗尽出口）；`-requester-rate`（配 `-requester-burst`）：单个请求方每秒建连速率上限，挡住"快开快关、绕过并发上限"的 churn 型滥用
- `-requester-bandwidth`（配 `-requester-bandwidth-burst`，字节/秒）：单个请求方吞吐上限（**熔断**）——超预算即切断当前传输、拒绝其新连接、并下调其声誉，字节预算随时间自动回补恢复；限的是**流量**而非连接数（目前覆盖 TCP 隧道）
- 出口默认对每条隧道加**超时**（收目标 10s、拨号 15s），防 slowloris/挂死拖垮
- **SSRF 防护（默认开启）**：出口默认拒绝连接私有/环回/链路本地地址（你的 LAN、`127.0.0.1`、云元数据 `169.254.169.254`）——IP 字面量在授权时拒，域名在拨号后按解析到的真实 IP 拒（防 DNS rebinding）；确需时用 `-allow-private-targets` 放开
- `-min-reputation`：拒绝声誉低于阈值的请求方（滥用者访问被封目标会被扣分，随后被各出口拒服务；默认禁用）
- `-audit-log <file>`：把每次服务/拒绝（时间/请求方/目标/结果）以 JSON Lines 追加落盘，供事后追责
- `-threat-feed <url|file>`：拉取已知恶意域名清单并入黑名单，定期刷新（`-threat-feed-interval`）

### 可观测性（指标）

加 `-metrics-addr` 让 node 以 JSON 暴露运维计数器（`/metrics`）。出口会记录服务量与**按原因分类的拒绝量**，便于确认限流/隔离/信任策略是否在按预期生效：

```bash
node ... -exit -metrics-addr 127.0.0.1:9090
curl -s http://127.0.0.1:9090/metrics
# {
#   "exit_served": 128,
#   "exit_denied:untrusted": 12,
#   "exit_denied:blocked-port": 3,
#   "exit_denied:requester-rate": 5
# }
```

### 配置文件

flag 较多时可把它们写进一个 JSON 文件，用 `-config` 加载——**键就是 flag 名**，命令行上显式给出的 flag 覆盖文件里的同名项（优先级：命令行 > 配置文件 > 默认值）。未知键会报错（防拼写错误）。node 与 coordinator 都支持。

```jsonc
// coord.json
{
  "addr": ":8080",
  "invite": "welcome",
  "uses": 100,
  "rate-limit": 5,
  "sybil-max-identities": 50
}
```

```bash
coordinator -config coord.json                 # 全部从文件读取
coordinator -config coord.json -uses 7         # -uses 覆盖文件里的 100
```

### 测试

```bash
go test ./...
```

完整测试指南（单元测试 + 多进程端到端出网 + 信任范围/出口保护/TUN 验证 + 一键脚本）见 **[docs/testing.md](docs/testing.md)**。

## 目录结构

```
cmd/coordinator   协调服务器入口（兼跑 Circuit Relay）
cmd/node          客户端核心入口（既是客户端也是出口）
internal/proxy    本地 SOCKS5 入口
internal/tunnel   隧道两端（出/入），解耦具体传输
internal/node     libp2p 接入：host、隧道、NAT 穿透、中继
internal/relay    Circuit Relay v2 中继服务
internal/coordinator  节点目录、邀请准入、信任图、HTTP 控制面
internal/trust    信任图（小网/背书/层级）、信任范围、两级声誉
internal/routing  选路引擎（地区 + 信任/延迟/负载综合排序）
internal/exit     ExitGuard 出口策略（信任范围/端口/域名/限额）
internal/tun      TUN 全局模式（tun2socks，-tags tun 构建标签）
pkg/protocol      隧道 wire 协议（目标地址编解码）
```

## 开发状态

- **M1** 骨架：本地 SOCKS5 + 两节点 libp2p 直连出网 ✅
- **M2** 组网：邀请准入 + 节点目录/发现 + NAT 穿透 + Circuit Relay 兜底 ✅
- **M3** 小网与信任：信任图/背书/信任层级/信任范围过滤/两级声誉 ✅（两级声誉的事件驱动与图形化首启动向导待 UI 阶段）
- **M4** 选路：地区 + 延迟/负载/信任度综合排序自动选最优 ✅
- **M5** 出口治理：ExitGuard（信任范围 + 端口/域名黑名单 + 并发限额） ✅（威胁情报接入与带宽熔断留待增强）
- **M6** 扩展：桌面 TUN 全局模式（`-tags tun`，基于 tun2socks）可编译 + 移动端接入设计文档 ✅（TUN 需管理员/wintun.dll 在真机验证；移动端待平台 SDK 落地）。详见 [docs/tun-and-mobile.md](docs/tun-and-mobile.md)

后续增强项（安全加固、UDP、持久化、UI、移动端落地等）见 **[docs/backlog.md](docs/backlog.md)**。抗国家级防火墙封锁的设计与分阶段路线图见 **[docs/anti-censorship.md](docs/anti-censorship.md)**。

> 注：真实跨 NAT 的打洞与 AutoRelay 自动预约需在两台异网机器上实测；本地回环已验证隧道与中继数据路径本身。

## 许可

[MIT](LICENSE) © 2026 WhatGate
