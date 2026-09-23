# WhatGate 功能说明

本页只回答一个问题：当前仓库已经能做什么？

状态：
- ✅ 已实现：代码与已有测试存在对应能力。
- 🧪 实验：代码存在，但有明确的真实环境 / 平台验证前提。
- 🗺️ 设计：主要存在于路线图或设计稿，不能当作现成功能。

## 1. 网络与成员

### 创建 / 加入

Coordinator 提供邀请制准入。第一台设备可以在“无成员”状态下完成一次性 founder bootstrap；之后加入必须使用邀请码。

邀请码有使用次数上限，由已准入成员创建，并记录 issuer 与 admission，形成“谁邀请谁”的准入链。

### 节点身份

节点使用 libp2p identity / PeerID。join、register 等身份敏感操作附带签名请求，服务端验证公钥对应 PeerID、签名动作和时间戳。

## 2. P2P 数据面

业务流量路径：

    应用
      ↓
    本地 SOCKS5
      ↓
    WhatGate tunnel
      ↓
    libp2p 直连
      └── 失败 → Circuit Relay v2
      ↓
    远程出口
      ↓
    目标站点

Coordinator 不承载代理业务流量，只提供发现和控制元数据。

节点启用 NAT port mapping、AutoNAT、hole punching；Coordinator 可同时运行 Circuit Relay v2 作为 fallback。

## 3. SOCKS5

默认本地代理地址：127.0.0.1:1080。

支持 TCP CONNECT 和 UDP ASSOCIATE。

本地入口使用 no-auth。生产环境应只监听 loopback，避免把本地代理暴露给局域网。

## 4. 地区与选路

节点注册 PeerID、multiaddr、region、WantExit 与当前 load。

客户端先按“不是自己 / WantExit / 地区 / trust scope”过滤，再排序。

默认排序为：**信任层级 → 延迟 → 负载**。

还支持 trust / latency / load 权重的加权组合。

地区只是选路标签，不是独立的地理真实性证明。

## 5. 信任圈与声誉

Trust scope：
- conservative：同组或被认可的信任关系
- open：允许整个网络中的候选出口参与

信任圈支持 group、group secret、跨组认可和 trust tier 查询。

声誉包含 peer reputation、group reputation、出口 outcome 上报、decay、持久化，以及可选的 minimum reputation 出口门槛。

声誉是信任、选路与 ExitGuard 的输入，不代表绝对可信度。

## 6. ExitGuard

ExitGuard 是共享出口的资源与安全策略层。

### 访问控制
- trust scope
- requester reputation
- 可选最低声誉

### 目标控制

默认阻止 SMTP 端口 25 / 465 / 587，并支持自定义端口、域名、IP / CIDR 黑名单和子域匹配。

默认拒绝 private / loopback / link-local / multicast / unspecified 目标，用于降低 SSRF 风险。

allow-private-targets 会关闭这层保护，应视为高风险运维开关。

### 资源限制
- 总并发
- 单请求方并发
- 单请求方建连速率
- 单请求方带宽

带宽超限会触发 breaker：当前传输停止，新连接被拒，并可以反馈为负面声誉事件。

### 审计与威胁情报

audit-log 记录时间、requester、target、结果与拒绝原因；threat-feed 从 URL 或文件加载恶意域名并周期性刷新。

## 7. DNS

SOCKS5 收到域名时，客户端不先解析，而是把 host:port 放进隧道，由出口发起连接。

因此：
- curl --socks5-hostname：出口侧解析
- curl --socks5：调用方可能先本地解析

出口 TCP 可以用 dns-server 指定解析器。UDP 路径有独立实现，详见 [dns.md](dns.md)。

## 8. Coordinator 控制面

当前 HTTP API：

| Endpoint | 作用 |
|---|---|
| POST /join | 加入网络 |
| POST /invite/create | 创建邀请码 |
| POST /register | 节点注册 / 刷新 |
| GET /directory | 节点 / 出口目录 |
| GET /relay | Relay 地址 |
| POST /group/join | 加入信任圈 |
| POST /group/endorse | 跨组认可 |
| GET /trust | 查询 trust tier |
| POST /report | 上报 outcome |
| GET /reputation | 查询 reputation |
| GET /groups | 查询成员组 |

写操作做身份签名校验，并可由 IP 级 rate limit 与 Sybil detection 做额外保护。

## 9. 控制面响应真实性

internal/discovery 已提供签名 envelope，用于 directory、relay、bootstrap、revocation 以及 Tier C credential / node record。

签名对象包含 type、serial、时间窗、revocation epoch 等字段；客户端可以 pin 公钥并拒绝低 serial 回滚。

因此“请求者是谁”和“Coordinator 返回的数据是否来自受信 key”是两个独立安全问题。

## 10. Coordinator 可用性

已实现多 coordinator URL、连接级 failover、已验证目录缓存、signed bootstrap out-of-band self-heal。

HTTP 业务错误不会被误判为“端点不可达”。

## 11. TUN

internal/tun 支持整机流量经过本地 SOCKS5 再进入 WhatGate。

构建需要：

    go build -tags tun ./cmd/whatgate

需要管理员/root；Windows 还依赖 wintun.dll。默认路由、IPv6、不同 NAT 条件应在目标平台上实测，因此归类为 🧪。

## 12. 私有认证 DHT

Tier C 已包含 offline root / issuer / member certificate、role authorization、node record、revocation checkpoint、private DHT member gating 和 Coordinator + DHT discovery merge。

它仍应标为 🧪。DHT 是发现冗余，不是授权来源；真实异网端到端验证仍是前提。

## 13. 桌面客户端

桌面端包装创建/加入网络、地区选择、trust mode、启停、当前出口、SOCKS5 地址、切换地区、共享出口、邀请码、信任圈、运行日志和高级管理。

桌面端负责核心进程生命周期，不要求普通用户手动访问本地 Web 控制接口。

## 14. 观测与运维

日志格式：log-format text|json。

指标：metrics-addr，/metrics 输出 JSON 运行指标。

/metrics 无鉴权，建议只绑定 loopback 或放在有鉴权的反向代理之后。
