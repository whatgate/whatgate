# B1 前置调研：libp2p `pnet`（私有网络 PSK）传输兼容性结论

> 目的：抗封锁路线图 Tier B1 想用 libp2p 的私有网络（`pnet`，预共享密钥 PSK）做**主动探测抗性**——没有密钥的探测者连握手都完不成。落地前，本文件先给出**「能不能上 pnet、能覆盖哪些传输」**的确定结论，避免基于过时资料做架构决策。
>
> **锁定版本**：`go-libp2p v0.48.0`（`go.mod`）。换版本须重跑本矩阵。

## 方法

1. **源码核查**（`$GOMODCACHE/github.com/libp2p/go-libp2p@v0.48.0`）。
2. **实测探针矩阵**：构造带/不带 PSK 的真实 libp2p 主机，跨连并观察结果（探针为一次性，已从仓库移除；结论见下表，可按“复现”一节重建）。

## 结论矩阵

| 场景 | 结果 | 依据 |
|---|---|---|
| `libp2p.New(PrivateNetwork(psk))`（默认传输） | ✅ **构造成功** | 设了 PSK 且未显式指定 `Transport` 时，libp2p **自动改用 `DefaultPrivateTransports`** = **仅 TCP + WS**（`defaults.go:53-58`；`config.go:160-165` 的 fallback）。实测主机只起 `/tcp/…` 监听。 |
| 显式 `Transport(quic.NewTransport)` + PSK | ❌ **构造报错** | `p2p/transport/quic/transport.go:74-76`：`len(psk)>0` 直接 `return errors.New("QUIC doesn't support private networks yet")`。实测 `libp2p.New` 失败。 |
| WebTransport + PSK | ❌ **构造报错** | `p2p/transport/webtransport/transport.go:98-100`：同样 `"WebTransport doesn't support private networks yet"`。 |
| 同 PSK，两节点走 **TCP** | ✅ **连通** | 实测 `Connect` 成功。 |
| 同 PSK，两节点走 **WS**（`/tcp/0/ws`） | ✅ **连通** | 实测成功，主机 addr 为 `/tcp/…/ws`。**这正是 A4 的 :443 伪装路径 —— 与 pnet 兼容**。 |
| **无 PSK** 节点 → 有 PSK 节点 | ❌ **被拒** | 实测 `Connect` "all dials failed"。**证实 pnet 确实把无密钥者挡在握手外**（B1 的反主动探测前提成立）。 |
| PSK + `ShareTCPListener` | ❌ **报错** | `config.go:480-481`：`"cannot use shared TCP listener with PSK"`。 |

## 裁决

**可以上 pnet，但只能覆盖 TCP + WebSocket；一旦启用 PSK，QUIC 与 WebTransport 整体不可用。** 这与我早前文档里“pnet 仅 TCP”的判断**方向一致**——机制上不是“被忽略”，而是**QUIC/WebTransport 构造即报错**，且 libp2p 通过 `DefaultPrivateTransports` 自动只保留 TCP+WS，故默认构造不会崩。

好消息：**pnet 与 A4 的 WS-on-:443 伪装路径兼容**（都是 TCP 系），二者可叠加。

## 对 WhatGate 的后果（落地 B1 前必须接受/处理）

1. **失去 QUIC/WebTransport**：NAT 穿透少一条 UDP 友好通道；DCUtR 打洞与 Circuit Relay v2 在 TCP 上仍可用，但需**在双异网机器实测** TCP-only+WS 下的打洞/中继成功率（本地回环不算数）。
2. **全网同一 PSK**：**每个节点 + 协调器同机中继**都必须带同一把 PSK，否则互不可连。中继主机当前用 libp2p 默认（含 QUIC）——启用 pnet 后中继也得改成 TCP+WS+PSK。
3. **低熵秘密在准入边界（评审 P0 复述）**：**不得**直接拿邀请码/小网口令当全网 PSK。应：高熵随机网络密钥作短期保护层 + 成员级凭据分发/封装/撤销；PSK 泄露=整网暴露、轮换=全员断连、无法撤销单成员。与安全评审 F5（口令改 HMAC 挑战）统一设计。
4. **pnet ≠ 完成的探测抗性（评审 P0 复述）**：连接超时/关闭时机/TLS 握手行为/包长时序仍可作探测预言机。B1 定位为**准入层**；真伪装/抗指纹仍需 A4（端口）+ B2（混淆/TLS 拟态）。
5. **单网 vs 分组**：全局单一 PSK 会把“大网”变成一个封闭圈，和现有“大网+小网联邦”模型冲突。若只想给**小网**上 pnet，需要多网络/多 PSK 或按组隔离 swarm——libp2p 单 host 只有一个 PSK，故“每组一 PSK”意味着多 host 或换用应用层挑战握手而非 pnet。**这是 B1 是否采用 pnet 的关键架构岔路。**

## 建议

- **短期**：A4 的 WS-on-:443 已落地且与 pnet 兼容，**先靠它 + A0/A1/A3 顶住**。
- **B1 采用与否是产品级决定**：pnet 能低成本拿到“无密钥者进不了握手”的反探测，但代价是丢 QUIC/WebTransport、全网单 PSK、与小网联邦模型冲突。**若采用**，先定“网络密钥 + 成员级凭据 + 撤销”方案（与 F5 合并设计），再改 node/relay 构造加 `PrivateNetwork`，并在双异网机实测 TCP+WS 下打洞/中继。
- **替代方案**：若不想被 pnet 的“全网单 PSK / 丢 QUIC”绑架，可考虑 **B2 应用层混淆/挑战握手**（在 libp2p 之上或 transport 层自定义），保留 QUIC 并支持按小网分域——但工程量更大。

## 复现

在 `internal/node` 放一个临时 `*_test.go`，用 `libp2p.New(libp2p.PrivateNetwork(psk), libp2p.ListenAddrStrings(...))` 构造带/不带 PSK 的主机并 `Connect`，覆盖上表七个场景（PSK 为 32 字节）；跑 `go test ./internal/node -run TestProbePNet -v` 观察。跑完删除（pnet 集成测试会绑定套接字，未采用前不宜入常驻测试集）。
