# C1.0 前置：kad-dht 私有认证 DHT 可行性矩阵（blocker matrix）

> 目的：C1（去中心化发现）设计稿把"私有*认证*成员 DHT"作为地基（[c1-decentralized-discovery.md](c1-decentralized-discovery.md)）。落地前，本文件先实测 **go-libp2p v0.48.0 + go-libp2p-kad-dht v0.42.1** 到底能不能支撑其硬性前提——**任一 blocker 不可实现，就停掉"私有成员 DHT"假设，改显式认证 discovery 服务/自定义协议**（Codex 评审 P1）。方法同 [pnet-compat.md](pnet-compat.md)：源码核查 + 一次性 hermetic 探针（localhost 多 host，探针跑完移除，见"复现"）。
>
> **锁定版本**：`go-libp2p v0.48.0`、`go-libp2p-kad-dht v0.42.1`（其 `go.mod` **恰好要求** libp2p v0.48.0，无强制升级）。换版本须重跑本矩阵。

## 结论矩阵（实测 2026-07-26）

| # | blocker 前提 | 实测结果 | 依据 / 备注 |
|---|---|---|---|
| **P1** | **协议前缀隔离公有 IPFS DHT** | ✅ **成立** | `dht.ProtocolPrefix("/whatgate")` 后：同前缀 peer 进路由表，**默认 `/ipfs` 前缀 peer 不进**（协议 ID 不匹配互不识别）。私有键空间隔离有效。 |
| **P2a** | **非成员挡在路由表外（PeerID 粒度）** | ✅ **成立** | `dht.RoutingTableFilter(func(_ any, p) bool)` 返回 false 的 PeerID **不进路由表**，即便同前缀已连接。这是"未认证不进路由表"的关键使能。 |
| **P2b** | **入站连接门控（进任何 RPC 前）** | ✅ **成立（须服务端强制）** | `libp2p.ConnectionGater` 的 `InterceptSecured` 返回 false：**出站拨号被干净拒绝**；**入站**——远端拨号方可能看到*瞬时 nil*，但**强制侧**（被连的成员）`Connectedness=NotConnected` 且**不进路由表**。**设计含义**：成员性强制必须落在**服务端 gater + RT filter + 拒绝 RPC**，**不能**假设"非成员根本连不上"。 |
| **P3** | **provider 语义 = 只给候选 PeerID** | ✅ **成立** | `Provide(cid)` + `FindProvidersAsync` 跨节点命中，**只返回 provider 的 PeerID，不带 payload**。证实**发现须两层**：FindProviders 得候选 → 再经认证协议直拉签名记录。 |
| **P4** | **PutValue 单值 → 多出口不能塞一个 key** | ✅ **成立** | 自定义 `dht.NamespacedValidator("wg", …)` 的 `Validate` 被调用（验签挂载点成立）；同 key 两次 `PutValue` 后 `GetValue` **只返回一个值**。证实**不能**用 `PutValue` 承载"同 region 多出口记录"（会互相覆盖）——须 provider 两层或受严格 validator 约束的多值索引。 |
| **P5** | **DHT 在 pnet 下可用** | ✅ **成立** | `libp2p.PrivateNetwork(psk)`（TCP+WS only，见 pnet-compat）+ DHT 同前缀：同 PSK 两节点**进路由表**。DHT 与 pnet 技术兼容——但是否启用 pnet 仍是 §"pnet×C1 先决"的产品决定，非本矩阵范围。 |

## 裁决

**私有认证成员 DHT 在 v0.48.0 + kad-dht v0.42.1 上可行（GO）**，六项硬前提全部满足。关键约束（写进设计）：

1. **前缀只是隔离，不是认证**——认证靠 **入站 gater（PeerID 粒度）+ RoutingTableFilter + 拒绝 RPC**，三者叠加。**成员*证书*（超出 PeerID 的授权）检查是握手后的应用层协议**，不是 gater 时点能做的（gater 在 `InterceptSecured` 只拿到 Noise 认证过的 PeerID）。→ 设计：**gater/RT-filter 用"已知成员 PeerID 集合"做粗准入，证书链在连接后由认证 `NodeRecord`/成员握手协议校验**（[c1 §5](c1-decentralized-discovery.md)）。
2. **入站强制在服务端**：不得假设非成员"连不上"；强制点是"被连成员不保留连接、不进 RT、不答 RPC"。
3. **发现两层**（P3+P4 共同证实）：`FindProviders`（粗索引，只给 PeerID）→ 认证协议直拉签名 `NodeRecord`。**禁止**把节点记录塞进 provider record 或 `PutValue`。
4. **验签挂载点成立**：`NamespacedValidator` 可承载 A0 `Signed` 校验（用于受约束的多值索引方案时）。

## 仍未在本矩阵覆盖（非 v0.48 API 可行性，另行验证）

- **NAT/CGNAT/relay × DHT 冷启动 + 跨网打洞/中继成功率**：需**双异网真机**（回环不算数），属 [c1 §11/§12](c1-decentralized-discovery.md) 上线门槛，非本地探针可判。
- **路由表污染 / resource manager 限额 / provider 重发布(reprovide)与 TTL 时序**：`RoutingTableFilter` 已给 PeerID 粒度防污染；`ResourceManager` 限额与 reprovide 间隔为**配置项**（存在性已知），其**时序**（provider 默认 ~22h 重发布、~24h 有效期）远长于分钟级地址寿命——**证实 provider TTL 与记录 TTL 必须解耦 + 查询后连通性挑战**（c1 §3）。
- **撤销陈旧处理**：应用层（签名 checkpoint + 陈旧上限），非 kad-dht 能力项。

## 对契约冻结的输入（C1.0 第二半）

本矩阵为"跨模块契约冻结"提供了确定的技术约束：
- **DHT 入站认证**：PeerID 粗准入（gater+RT filter）+ 连接后成员握手/证书校验协议——**冻结为两段**，而非单点。
- **NodeRecord 拉取协议**：独立于 provider 层的**认证自定义 stream 协议**（复用 A0 `Signed` + 成员证书链 + 连通性挑战 nonce）。
- **pnet 模式**：技术上可叠加；**是否启用**须在 C1.0 前与 B1 决策合并拍板（当前 B1 暂缓 → 默认**不启用 pnet**，靠 DHT 入站证书认证；若未来启用 pnet，作传输层门槛，**不替代**证书/撤销/配额）。

## 复现

在 `internal/node` 放临时 `c1_spike_test.go`：用 `libp2p.New(ListenAddrStrings("/ip4/127.0.0.1/tcp/0"), …)` + `dht.New(h, dht.Mode(dht.ModeServer), dht.ProtocolPrefix("/whatgate"), …)` 构造多 host，覆盖上表 P1–P5（前缀隔离、`RoutingTableFilter`、`ConnectionGater`+连通性/RT 检查、`Provide/FindProvidersAsync`、同 key 双 `PutValue`+`GetValue`、`PrivateNetwork(psk)`+DHT）；`go test ./internal/node -run TestC1DHTBlockerMatrix -v` 观察日志。跑完删除（探针绑套接字，C1 正式落地前不入常驻测试集）。依赖：`go get github.com/libp2p/go-libp2p-kad-dht@v0.42.1`（正式落地 C1.1 时再入 go.mod）。
