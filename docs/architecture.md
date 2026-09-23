# WhatGate 架构说明

## 1. 核心原则：控制面与数据面分离

WhatGate 有两个平面：

| 平面 | 负责 | 是否承载代理业务流量 |
|---|---|---|
| Control Plane | Coordinator：准入、目录、信任、声誉、Relay 元数据 | **否** |
| Data Plane | libp2p：客户端与出口之间的加密隧道 | **是** |

Coordinator 不承载代理业务流量；它主要影响后续发现、刷新和切换。

## 2. 组件关系

```text
Browser / App
    ↓
SOCKS5
    ↓
WhatGate Tunnel
    ↓
libp2p Node ─────────────→ Remote Exit ─→ Internet
    │                         ↑
    └──── direct preferred ───┘
    │
    └──── Circuit Relay v2 fallback

Desktop UI ─→ whatgate core ─→ local control API
Node ───────→ Coordinator control plane
```

## 3. 客户端请求生命周期

### 加入

```text
Desktop / CLI
    ↓ signed /join
Coordinator
    ↓
invite / founder bootstrap
    ↓
admission state
```

### 注册

节点向 Coordinator 注册：

- PeerID
- multiaddrs
- region
- WantExit
- load

目录条目带 TTL；停止刷新后自动过期。

### 发现与选路

客户端：

1. 排除自己
2. 过滤 WantExit
3. 过滤 region
4. 过滤 trust scope
5. 根据 trust / latency / load 排序
6. 建立到出口的数据面连接

### 建隧道

```text
SOCKS5 CONNECT host:port
    ↓
tunnel client
    ↓
libp2p stream
    ↓
WriteTarget(host:port)
    ↓
remote exit
    ↓
ExitGuard.Authorize
    ↓
dial target
    ↓
双向 relay
```

出口授权失败发生在目标拨号之前。

## 4. UDP 数据路径

```text
SOCKS5 UDP ASSOCIATE
    ↓
WhatGate UDP stream
    ↓
datagram(target, payload)
    ↓
exit UDP socket per target
    ↓
target
```

TCP 与 UDP 都使用 ExitGuard 的信任、目标和资源控制；UDP 会在域名解析后再次检查实际 IP，避免 DNS 指向内网目标。

## 5. NAT 与中继

internal/node 启用：

- NAT port mapping
- AutoNAT
- hole punching / DCUtR
- 可选 static Circuit Relay v2

直连和中继属于数据面问题，与 Coordinator 的发现职责分开。

## 6. 信任与声誉

internal/trust 维护：

- group membership
- endorsement
- trust tier
- peer reputation
- group reputation

选路使用 trust tier；ExitGuard 使用 trust scope 与 reputation 决定是否服务。

因此存在两个不同决策点：

**选路：** 哪个出口值得尝试？

**授权：** 这个请求方现在能不能使用我的机器？

## 7. ExitGuard

出口请求大致经过：

```text
Requester
  ↓
Trust scope
  ↓
Minimum reputation
  ↓
Blocked port/domain/IP
  ↓
Private-target SSRF guard
  ↓
Connection / requester rate limits
  ↓
Bandwidth breaker
  ↓
Dial target
  ↓
Relay bytes
```

策略拒绝尽可能发生在最早阶段，以减少资源占用。

## 8. 控制面真实性

请求鉴权和响应鉴权是两套机制。

### 请求：internal/authn

证明：

> 请求由对应 PeerID 的私钥持有者签发。

### 响应：internal/discovery

证明：

> directory / relay / bootstrap 等对象由被 pin 的控制面 key 签发。

签名对象包含 type、serial、时间窗、revocation epoch 等字段；客户端可以维护 serial floor 防止旧对象回滚。

## 9. Coordinator 可用性

```text
Coordinator A ─┐
Coordinator B ─┼─→ endpoint failover
Coordinator C ─┘
        ↓
verified directory
        ↓
local cache
        ↓
signed bootstrap URL
```

只有连接级故障触发 failover；收到 HTTP 业务响应后不会把它误判为“端点不可达”。

## 10. DHT / Tier C

实验性私有 DHT 是 Coordinator 的发现冗余，而不是新的授权根。

```text
DHT candidate
    ↓
signed NodeRecord
    ↓
member certificate chain
    ↓
revocation checkpoint
    ↓
verified / eligible / reachable
```

Coordinator 仍是 trust / reputation 的权威来源。DHT-only 候选不会因为“被发现”自动获得推荐资格。

## 11. 桌面端

Avalonia 桌面客户端负责：

- 保存用户配置
- 启停 whatgate
- 创建网络时启动随包 Coordinator
- 轮询核心状态
- 调用本地控制 API
- 提供连接、地区、出口和信任圈 UI

核心网络逻辑仍在 Go 进程中，UI 不复制网络实现。

## 12. 本地控制接口

-web 提供本地 dashboard / 控制接口；桌面客户端使用它管理状态、地区、出口、邀请码和 group 操作。

普通用户不需要手动访问 127.0.0.1:7070。

## 13. 安全边界

当前重点保护：

- 节点身份不被冒充
- 控制面响应不被任意协调器伪造
- 共享出口降低 SSRF、端口滥用和带宽耗尽风险
- NAT 用户拥有直连 + relay fallback

当前没有声称已经解决：

- 完整主动探测抗性
- DPI 不可区分
- 流量关联防御
- 所有公网 NAT 条件下的成功率
- 所有平台的 TUN 默认路由正确性

这些边界与 security-review.md、anti-censorship.md 一致。
