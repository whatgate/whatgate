# WhatGate Backlog（待增强清单）

M1–M6 的核心链路已完成（见 [architecture.md](architecture.md) 路线图）。本清单汇总后续增强项，供规划与贡献者认领。

**优先级**：`P0` 关键（安全/正确性底线）· `P1` 重要（可用性/完整性）· `P2` 改进（体验/优化）
**状态**：`todo` 待做 · `partial` 部分已有基础

---

## A. 安全与信任加固（项目立身之本，优先）

| 优先级 | 项 | 说明 |
|---|---|---|
| P0 | ~~**签名注册**~~ ✅ **已完成** | 节点用 libp2p 私钥签名 register/join（`internal/authn`），协调器校验公钥→PeerID 一致 + 签名有效 + 时间戳新鲜（防重放）。入群签名见下条（小网操作鉴权）。 |
| P1 | ~~**小网操作鉴权**~~ ✅ **已完成** | 入组需签名(证明本人 peerID，防冒充)+ 小网口令(防陌生人凭名字混入)；背书需签名且发起者为 fromGroup 成员。负面用例：错口令 403 / 冒充 401 / 非成员背书 403。 |
| P1 | ~~**声誉事件驱动 + 衰减 + 持久化**~~ ✅ **已完成** | 事件驱动 ✅（`trust.Outcome`，出口 `/report`）；持久化 ✅（Phase 1.5）；衰减 ✅（`Reputation.Decay`，coordinator `-reputation-decay`，分数周期性向 0 回归）。 |
| P1 | ~~**ExitGuard 声誉门槛**~~ ✅ **已完成** | `Policy.MinRequesterReputation`，出口授权时查请求方声誉、低于阈值即拒（`-min-reputation`，默认禁用）。与 2.6 成闭环：滥用被扣分后被各出口拒服务（e2e `scripts/e2e-reputation.ps1` 验证）。 |
| P2 | ~~**抗女巫 / 协调器限流**~~ ✅ **已完成** | **频率限制 ✅**：按客户端 IP 令牌桶（`internal/ratelimit`）限流变更类端点（join/register/group/report），coordinator `-rate-limit`/`-rate-burst`，超限 429。**异常行为检测 ✅**：`internal/anomaly` 滑窗统计每 IP 建立的**不同身份数**（join 时按 PeerID 去重），超阈值即隔离该 IP 后续 join（coordinator `-sybil-max-identities`/`-sybil-window`，默认禁用）——补住"低速持久注册攒 Sybil 舰队"这一令牌桶挡不住的模式。诚实边界：按 IP 计，CGNAT/代理下阈值须放宽。 |

## B. 出口治理增强（ExitGuard）

| 优先级 | 项 | 说明 |
|---|---|---|
| P1 | ~~**连接留痕持久化（可追溯审计）**~~ ✅ **已完成** | 出口 `-audit-log` 把"时间/谁/连了什么/结果(served/denied+原因)"以 JSON Lines 追加落盘（`internal/audit`），配合邀请链做事后追责。 |
| P1 | ~~**威胁情报域名源**~~ ✅ **已完成** | 出口 `-threat-feed`(url/文件, hosts 或纯域名格式)启动拉取并入黑名单、定期刷新(`internal/threatfeed`)。 |
| P2 | **带宽级限额与熔断** | 当前限并发连接数；补带宽配额 + 异常流量模式自动熔断并降请求方声誉。 |
| P2 | ~~**按请求方限速**~~ ✅ **已完成** | 出口 ExitGuard 双重限单请求方：并发上限（`MaxConnsPerRequester`，`-max-conns-per-requester`）+ 建连速率令牌桶（`RequesterRatePerSec`/`RequesterBurst`，`-requester-rate`/`-requester-burst`，复用 `internal/ratelimit`），后者挡住快开快关、绕过并发上限的 churn 型滥用（超限 `ErrRequesterRateLimited`）。默认全禁用。 |

## C. 组网 / 协调 / 中继

| 优先级 | 项 | 说明 |
|---|---|---|
| P1 | ~~**状态持久化**~~ ✅ **已完成** | 邀请/准入/信任图/小网口令/声誉快照到 JSON 文件（`internal/persist`，原子写），变更即存、启动即载（coordinator `-state`）。目录仍短时（节点续注册自愈）。规模变大可换 bbolt。 |
| P1 | **跨 NAT 真实验证** | 用两台不同网络的机器实测 DCUtR 打洞与 AutoRelay 自动预约（回环无法覆盖）。 |
| P2 | **协调器高可用 / 去中心目录** | 首版单实例；后续多实例或渐进式 DHT 目录，消除单点。 |
| P2 | **中继资源限制** | 给 Circuit Relay 加带宽/时长/预约数配额，防中继被滥用。 |

## D. 传输 / 选路

| 优先级 | 项 | 说明 |
|---|---|---|
| P1 | ~~**UDP 支持**~~ ✅ **已完成** | SOCKS5 UDP ASSOCIATE + libp2p UDP 数据报隧道（`pkg/protocol` 帧、`node` UDPSession/出口转发、`proxy` UDP 中继），DNS-over-UDP 真实出网验证。UDP 出口已接 ExitGuard（与 TCP 共用授权：信任范围/声誉/端口/域名/并发）。 |
| P2 | ~~**选路精细化**~~ ✅ **已完成** | **可配加权综合评分 ✅**：`routing.RankExitsWeighted`（`Weights{Trust,Latency,Load}`，综合分排序 + 字典序 tie-break，仍按 scope 过滤）；node `-rank-{trust,latency,load}-weight`（任一非零启用，默认字典序不变）。**延迟 EWMA 平滑 ✅**：`routing.LatencyTracker` 跨重发现轮对每出口 RTT 做指数加权移动平均，单个慢探针被阻尼不再抖动选路；node `-latency-ewma-alpha`（默认 0.3，置 1 关闭平滑=旧行为）；不可达轮用哨兵排末但不污染平滑历史，恢复即回。 |
| P2 | **DNS 策略** | 明确并可配 DNS 解析位置（出口侧解析利于解锁）；文档化防 DNS 泄漏。 |

## E. TUN 全局模式（桌面）

| 优先级 | 项 | 说明 |
|---|---|---|
| P1 | ~~**自动路由 / 排除自身流量**~~ ✅ **代码完成（待真机验证）** | `-tun-auto-route`：自动分配 TUN IP、以两个 /1 半段抢占默认路由（不删系统默认，便于还原）、把协调器 + `-connect` 出口的 IP 钉在物理网关（防隧道抓到自身包回环），退出时自动还原。路由命令生成为纯逻辑并单测（Win/mac/Linux 三平台 `internal/tun/route.go`）；执行器 `ApplyUp/ApplyDown` + 网关自探测 `DefaultGateway`。真机需管理员/root 实测。 |
| P2 | **UDP/DNS 分流、IPv6** | 全局模式下的分流规则与 IPv6 支持。 |
| P2 | **驱动打包与安装器** | 打包 `wintun.dll`、各平台安装/权限引导。 |

## F. 移动端（Android / iOS）

| 优先级 | 项 | 说明 |
|---|---|---|
| P1 | **gomobile 瘦封装 + bind** | 暴露 `Start(configJSON, tunFD)/Stop()`，产出 Android `.aar` / iOS `.xcframework`（复用核心 + tun2socks fd 后端）。 |
| P1 | **系统 VPN 接入** | Android `VpnService` / iOS `NEPacketTunnelProvider`（iOS 需 Network Extension 权限与开发者账号）。 |
| P1 | **移动 UI 外壳** | 地区选择、出口开关、小网管理、首启动信任向导。 |

参见 [tun-and-mobile.md](tun-and-mobile.md)。

## G. 桌面 UI 与体验

| 优先级 | 项 | 说明 |
|---|---|---|
| P1 | **桌面外壳** | 本地 Web 控制台：状态面板 ✅ + 切换出口地区 ✅ + 开关出口 ✅ + 首启动信任向导 ✅ + **小网管理**（加入/创建/背书/我的小网列表）✅。**剩：系统托盘**（原生集成，可选）。 |
| P1 | **图形化首启动信任向导** | 解释"成为出口/用陌生出口"的风险，让用户选信任范围档位（CLI 现以 `-trust-scope` 无默认体现）。 |
| P2 | ~~**配置文件支持**~~ ✅ **已完成** | node/coordinator `-config <file.json>`：键即 flag 名，命令行覆盖文件（优先级 命令行 > 文件 > 默认），未知键报错防拼写。通用实现（`internal/config.ApplyFile` 用 `flag.Visit` 判显式设置，无需逐 flag 接线）；JSON 而非 YAML 以零新增依赖（合分发简易性 + 复用 `internal/persist` 约定）。 |
| P2 | **小网管理 UX** | 退组、移除成员、撤销背书等操作与界面。 |

## H. 工程 / 发布 / 运维

| 优先级 | 项 | 说明 |
|---|---|---|
| P1 | ~~**CI**~~ ✅ **已完成** | `.github/workflows/ci.yml`：push/PR 自动 `gofmt`/`vet`/`test`/默认构建 + `-tags tun` 构建。 |
| P1 | ~~**交叉编译与发布**~~ ✅ **已完成（代码签名待做）** | `scripts/build-release.sh` 一键交叉编译六平台（Win/mac/Linux × amd64/arm64）的 coordinator/node/node-tun，打包 + SHA256SUMS；`.github/workflows/release.yml` 在推 `v*` tag 时自动构建并 `gh release create`。二进制经 `-ldflags -X main.version` 版本戳（`-version` 可查）。**剩：代码签名/公证**（Windows Authenticode、macOS notarization）。 |
| P1 | **安全评审** | 对隧道、协调协议、ExitGuard 做一次专门的安全审查。 |
| P2 | **可观测性** | 结构化日志 + 指标（连接数、选路、拒绝原因等），便于运维。 |

---

> 说明：本清单不含已完成的 M1–M6 核心功能，只列增强/收尾项。认领时建议每项单独开 issue，并遵循仓库的 TDD 约定（纯逻辑先写测试）。

---

## 建议实施顺序（P0/P1）

排序原则：**先立工程护栏 → 打牢身份/信任底座（改协议的早做，避免返工）→ 依赖它的治理闭环 → 独立轨（传输/TUN）可并行 → 最外层 UI/移动端 → 收尾安全评审**。

### Phase 0 — 工程地基（成本低、护栏优先）
1. ~~**CI**~~ ✅ 已完成。
2. ~~**交叉编译 + Releases**~~ ✅ 已完成（`scripts/build-release.sh` + `release.yml`，推 tag 自动发六平台）。**Phase 0 全部完成。** 这解锁了把二进制发到真机做"跨 NAT 实测/TUN 真机验证"。

### Phase 1 — 身份与信任底座（安全根基，越早越好）
3. ~~**签名注册（P0）**~~ ✅ 已完成（register/join 已验签）。
4. ~~**小网操作鉴权**~~ ✅ 已完成（入组签名+口令；背书需成员）。
5. ~~**状态持久化**~~ ✅ 已完成（JSON 快照，coordinator `-state`）。**Phase 1 身份/信任底座全部完成。**

### Phase 2 — 声誉与出口治理闭环（← 当前阶段）
6. **声誉事件驱动** ✅ 已完成（served/blocked → 调整 peer+小网声誉，出口经 `/report` 上报）。剩衰减（周期 Decay）。
7. **ExitGuard 声誉门槛** ✅ 已完成（`-min-reputation`；与 2.6 成闭环）。
8. **连接留痕持久化** ✅ 已完成（出口 `-audit-log`，JSON Lines）。
9. **威胁情报域名源** ✅ 已完成（`-threat-feed`）。
10. **声誉衰减** ✅ 已完成（coordinator `-reputation-decay`）。

**Phase 2 出口治理闭环 100% 完成**（准入→身份→信任→声誉→出口门槛→留痕→威胁情报→衰减）。**下一大块=Phase 3（传输/全局模式）或 Phase 4（UI/移动端）。**

### Phase 3 — 传输与全局模式（独立轨，可与 Phase 2 并行）
10. **UDP 支持** ✅ 已完成（SOCKS5 UDP ASSOCIATE + UDP 隧道 + UDP 出口 ExitGuard，DNS 出网验证）。
11. **TUN 自动路由 / 排除自身流量** ✅ 代码完成（`-tun-auto-route`，三平台路由规划已单测；待管理员真机实测）。
12. **跨 NAT 真实验证** ← **下一项**。有两台异网机器即可做（Phase 0 发布后）。

### Phase 4 — 面向用户（← 当前阶段）
13. 桌面外壳：状态面板 ✅ + 切换出口地区 ✅ + 开关出口 ✅ + 首启动信任向导 ✅ + 小网管理 ✅。**Web 控制台功能完整；剩系统托盘（原生，可选）。**
14. **移动端** — gomobile 绑定 + Android VpnService / iOS NEPacketTunnelProvider + UI（需平台 SDK）。

### 收尾
15. **安全评审** — 在 Phase 1–2 落地后，对隧道/协调协议/ExitGuard 专项审查。

**关键路径**：3 → 5 → 6 → 7（身份→持久化→声誉→出口门槛）是环环相扣的主线；Phase 3、Phase 4 是可并行的独立分支。
