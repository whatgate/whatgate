# C1 去中心化发现设计稿（私有认证 DHT，协调器权威 + DHT 冗余）

> **状态**：设计评审稿（**已经 Codex `/cx-plan` 对抗评审并折叠修正**，见 §14）。**本文件不含实现代码。**
>
> **锁定版本**：`go-libp2p v0.48.0`（`go.mod`）。kad-dht / rendezvous 目前**尚非依赖**。换版本须重跑 C1.0 blocker 矩阵。
>
> 上位文档：[anti-censorship.md](anti-censorship.md)（本稿展开其 Tier C / C1）。相关：[pnet-compat.md](pnet-compat.md)（B1 调研范式）、[security-review.md](security-review.md)（F1 SSRF / F5 挑战 / F2 声誉）。

## 决策摘要

| 维度 | 决策 | 含义 |
|---|---|---|
| **DHT 形态** | **私有*认证* DHT** | 自定义 protocol 前缀**只是隔离，不是访问控制**；真正的准入是**入站连接门控**——DHT stream 在进入 peerstore/路由表/任何 RPC 前，必须出示有效成员证书（§5）。bootstrap 由带**信任包**的 C2 清单 + 内置签名种子供给（§9）。 |
| **去中心化程度** | **协调器权威 + DHT 冗余（双轨）** | 准入/信任/声誉以协调器为**权威**；DHT 只作**冗余发现索引**，**绝不作授权来源**，**永不提供"推荐/新鲜声誉"**（§4 四状态合并）。诚实定位为**"多点控制面"**，且在高 CGNAT 下**只能标注为弱冗余**直到容量门槛达标（§8）。 |
| **签发根** | **离线根 + 在线受限 issuer**（评审 P0 强制，一期即做） | 在线 issuer 子证书由**离线根**签发、短期、用途受限，只能签**普通成员**；**出口/relay 授权须离线根或双人审批**。在线 join 服务单签**不可**获得出口权。含 issuer 撤销/轮换/append-only 审计/客户端 issuer allowlist。 |
| **PeerID 轮换** | **v1 禁止**（评审 P0） | 保持长期 transport PeerID（=身份密钥）；只轮换**地址 + 广告 nonce + 短期广告签名密钥**。身份迁移是**独立协议项目**，不放进 C1。 |
| **rendezvous / 身份轮换** | **移出 v1 核心（已裁决 2026-07-26，采纳 Codex）** | v1 冻结在"**认证 DHT provider 索引 + 认证记录拉取 + 四状态合并 + 治理**"；rendezvous 与 PeerID 轮换作**默认关闭的独立后续能力**（各自先过独立威胁模型/量化验收）。**枚举面加固（§5/§7 的 epoch capability / 最小字段 / 短期地址 / 限速）保留在 v1 核心。** |
| **跨模块契约** | **C1.0 已冻结**（✅ 见 §15） | 证书/撤销 checkpoint 格式、bootstrap 信任包、DHT 入站认证（两段）、挑战绑定主体、pnet 模式、版本协商 + 降级拒绝——已冻结。 |

## 0. 目的 / 非目的

**目的**：去掉"单一协调器被封 = 全网发现瘫痪"命门，把出口发现做成**冗余多点控制面**。

**非目的**（明确划界）：
- **不解决数据面探测抗性**（Tier B）；**DHT 查询/记录流量本身可指纹**，需与 B2 协同。
- **不替换邀请制准入**；成员资格源于邀请 + **离线根链**签发的成员证书。
- **不追求"无任何单点"**：bootstrap/relay/入站 DHT server 仍是少数高价值点。
- **不承诺"恶意*已授权*成员无法枚举其被授权 scope"**——这是私有认证 DHT 的**已知残余风险**（§7）。

## 1. 现状与复用（file:line）

- **libp2p host**：`internal/node/node.go:70-98`（NAT/AutoNAT/HolePunching + `EnableAutoRelayWithStaticRelays`）。**当前无 DHT**；C1 在此挂 DHT + **入站 ConnectionGater**（§5）。
- **A0 签名信封**：`internal/discovery/signed.go`（`Signed` + 域分隔 `canonical` + `Verify`：钉扎 + 反回滚 floor + 过期 + 类型）。**复用**：新增 `TypeMemberCert`、`TypeNodeRecord`、`TypeRevocationCheckpoint`、`TypeBootstrapBundle`。
- **请求签名**：`internal/authn`（PeerID + action + 时间戳；协调器侧重放去重 `checkAuth`）。挑战响应沿用**成员私钥绑定签名**（§8，非共享 HMAC）。
- **准入**：`internal/coordinator`（invite/join）。成员证书签发挂在准入后，但**签发密钥是离线根派生的在线受限 issuer**（决策摘要）。
- **SSRF 既有防线**：`internal/exit`（F1，出口拨*目标*的私网/环回/metadata 拒绝）——C1 复用其思路，但**新增对 DHT 广告的 peer 地址的拨号策略/地址 gater**（§4 拨号策略，另一处攻击面）。
- **C2 带外引导**（`internal/coordinator/bootstrap.go`）：`BootstrapList` 现只含 `endpoints`；C1 **扩展为 `TypeBootstrapBundle`**（钉扎 bootstrap PeerID + 地址 + 签名描述 + **证书状态快照** + DHT 种子），带签名版本/过期/撤销/多渠道。

## 2. 诚实定位与威胁（承接评审）

1. **前缀 ≠ 访问控制**（P0）：协议前缀可从二进制/流量/被攻陷成员取得。**必须入站认证**，否则对手仍可探测、灌路由表、跑 `FIND_NODE/GET_PROVIDERS`、耗资源。**"查询结果最终不采信" ≠ "DHT 私有"。**
2. **发现 ≠ 信任 → 四状态**（P0）：`authorized`（有有效证书）/`eligible`（能力+未撤销+新鲜度内）/`recommended`（**权威声誉**）/`observed-reachable`（本地连通挑战）。**DHT 只能抬升 authorized/eligible/本地观测，绝不补 recommended。** 无权威新鲜声誉时，默认只允许**用户显式 pin 的出口**或**严格受限的应急 profile**。
3. **Sybil（P0）**：邀请/前缀/共享秘密**不解决女巫**。每成员独立**离线根链**证书 + 每主体配额 + **绑私钥的挑战**（§8）。攻陷单成员**只**能造其配额内记录且**可定向撤销**。
4. **枚举面（P0）**：私有认证 DHT 在**成员证书/发现 capability 泄露后**放大枚举。→ 每 epoch、每角色/region 的**派生发现 capability**（非低熵稳定 region 键）、最小 payload、限速；把**可枚举性**列为量化红队项。承认**恶意已授权成员**仍可枚举其 scope（残余风险）。
5. **k-近邻劫持（P0）**：低熵稳定的 region 键会被"离线生成靠近该键的 PeerID"占据 k-近邻，应用层发布配额挡不住路由表先占。→ epoch 化不可公开 capability namespace + 入站成员准入 + 身份生成成本 + **关键 region 用固定可信观察节点/多独立路径**，不只靠 XOR 近邻。
6. **rendezvous 半中心化**（P1）：运营者可实时镜像/关联注册与查询，**"不留日志"不是安全控制**。移出 v1 核心（决策摘要）。

## 3. 架构：双轨发现

```
权威面（协调器，不变）                冗余发现面（私有认证 DHT）
────────────────────                 ──────────────────────────
离线根 → 在线受限 issuer  ──签发──▶   成员证书(TypeMemberCert，客户端持有，不上 DHT)
join / 信任 / 声誉                    ┌ 入站门控: 无有效证书者不进 peerstore/路由表/RPC
A0 目录/relay(权威, 带信任层级)       │ 出口: Provide 到"每epoch派生的角色/region capability CID"
签名撤销 checkpoint  ──分发(带外+DHT)─▶│ 查询者: FindProviders → 候选 PeerID
                                     │ 再经*认证的 NodeRecord 协议*向候选拉取签名记录
                                     └ 四状态验证 + 连通性挑战 → 连接(复用 NAT/relay)
```

- **provider ≠ 记录层**（P0，实现正确性）：`Provide/FindProviders` 只绑定"PeerID 是某 CID 的 provider"；`PutValue/GetValue` 按键选**单一最佳值**会让同 region 多出口互相覆盖。**故两层拆分**：① `FindProviders(capability-CID)` 只得**候选 PeerID**；② 经**认证的自定义 `NodeRecord` 协议**向候选**直接拉取**其签名记录（或实现受严格 validator 约束的多值索引）。**禁止把节点记录塞进 provider record。** Kademlia 语义（`ADD_PROVIDER/GET_PROVIDERS/PUT_VALUE`）作 C1.0 硬验证项。
- **provider TTL 与记录 TTL 解耦**（P0）：provider 只是**粗索引**（刷新周期远长于分钟级地址寿命，会持续返回离线/换址 PeerID）；查询后**必经**短时、带 nonce 的记录拉取 + **连通性挑战**才采信。限制每出口的 region 数与 refresh 预算，避免 `Provide` 造成可观测流量尖峰。

## 4. 记录 / 证书 / 撤销 / 拨号（发现≠信任落成可执行）

**签名对象（复用 A0 `discovery.Signed`，域分隔 type）：**

- **成员证书 `TypeMemberCert`**：**离线根 → 在线受限 issuer** 签发。payload：成员 PeerID、issuer 标识、角色/能力位（普通/出口/relay——**出口/relay 须离线根或双人审批**）、有效期、单调序号、撤销 epoch。**不上 DHT**（减枚举），客户端本地持有并在**入站握手**与**记录背书**时出示。
- **节点记录 `TypeNodeRecord`**：出口自签，payload：PeerID、当前 addrs、角色/region、**分钟级**有效期、**严格递增 generation**、payload hash、签名时间窗，内嵌/引用成员证书。
- **撤销 checkpoint `TypeRevocationCheckpoint`**（P0，撤销闭合）：由根签名，含 `thisUpdate/nextUpdate`、**单调版本**、最大可接受陈旧度、根轮换规则。带外（C2）+ DHT 冗余分发。**离线超过陈旧上限**：不区分"未撤销/被封锁"，**只允许显式受限应急 scope**，**不继续建普通出口隧道**（防 fail-open 与全网误拒）。

**四状态验证链（任一失败 → 安全拒绝）**：
1. `Signed.Verify`：记录由声称 PeerID 签、类型正确、未过期、**generation 单调**（反回滚，主键；签名时间仅作界，不依赖墙钟排序）。
2. **成员证书链**：内嵌证书由**受信 issuer（客户端 allowlist，根可验）**签、绑同一 PeerID、未过期、**未被撤销**（对最新可信 checkpoint，且在陈旧上限内）。
3. **能力**：证书声明角色 ∧ 记录 region/能力自洽。
4. **连通性挑战**：候选地址经**短时带 nonce 的握手证明**可连（防挂旧址）。
5. **拨号策略（P0，SSRF）**：验证前后都过**地址 gater**——拒 loopback/link-local/RFC1918/ULA/metadata/非允许 transport/异常端口；**只拨由已认证 PeerID 在握手证明的地址**；设每候选/每前缀/每轮查询的**拨号预算**。
6. **四状态合并**：DHT 来源候选给到 `authorized/eligible/observed-reachable`；**recommended 只能来自权威面**。权威失联 → 仅 pin/应急。

**拜占庭 equivocation（P0，双轨一致性）**：为每逻辑出口定义 issuer/主体/epoch/generation/payload-hash/签名时间窗；**同 generation 不同 hash = equivocation → 隔离该主体**。合并用**确定性状态机**；缓存**绑定"验证时所用的撤销 checkpoint 版本"**，不是单一 `floor`。

**单写者（P1）**：多进程/多设备同私钥会争 generation。→ **每证书单一广告 writer** 或根签发的 **writer lease**；持久化 epoch/generation + 崩溃安全原子提交。

## 5. 私有认证 DHT：入站门控（核心机制）

- **协议前缀**：`dht.ProtocolPrefix("/whatgate/kad")` 仅做**键空间/协议隔离**（非安全边界）。
- **入站认证门控（P0，最关键）**：在 DHT handler **之前**用 libp2p `ConnectionGater` + 一次成员握手：验证对端**当前成员证书**、证书状态（未撤销）、角色、每身份配额；**未认证连接不进 peerstore、不进路由表、不响应任何 DHT RPC**。
- **模式**：有公网入站能力的成员跑 **DHT server**，NAT 后成员跑 **client 模式**（不承载路由表，减枚举与负载）。
- **发现 namespace**：出口 Provide 的 CID = **每 epoch、每角色/region 的派生 capability**（由成员凭证/短期授权 token 派生），**非低熵稳定 region 明文键**（§2.5 k-近邻防护）。
- **不加密协议前缀**（P1）；**不做全体共享组密钥加密**（泄露一成员即失效、且挡不住已授权成员枚举）。capability 泄露只轮换**受影响 capability**，不轮换全网 transport。

## 6. rendezvous（移出 v1 核心，已裁决）

- Codex P0/P1：rendezvous 引入**半中心化在线情报库**，且**不能弥补 DHT 信任缺陷**。**v1 不做**（用户已裁决 2026-07-26 采纳），作**默认关闭的独立后续能力**：需成员认证、每成员/角色 **blind capability**、注册/发现权限分离、固定返回上限、抗关联 padding、运营者视为可观察对手；**完成独立威胁模型 + 量化匿名度前不接生产发现**。

## 7. 枚举面（残余风险，量化验收）

- 每 epoch/角色/region 派生 capability + 最小 payload + 短期地址/广告 nonce + 限查询范围/速率（指数退避）。
- **明确残余风险**：**恶意已授权成员可枚举其被授权 scope**——不可完全解决；量化"给定一个泄露成员证书能枚举多少成员/出口/在线/关联"作红队门槛。

## 8. Sybil / eclipse / relay 治理

- **Sybil 成本**：邀请 + **每成员离线根链证书**（可归因/撤销）+ 对 provider 发布/记录拉取/拨号并发的**每主体配额** + **挑战**——挑战**必须绑成员私钥**（请求签名 + 服务端 nonce）或 issuer 发放的**不可转让限额 blind token**；**禁止全体共享 HMAC 作身份**（P1，泄露一成员即可离线伪造）。PoW/延迟/带宽成本与证书配额**分层**。
- **Eclipse**：多路径/多近邻查询 + 跨源一致性——但**独立性须定义**（P1）：不同 bootstrap 签名组 / ASN / 地理 / 运营者 / 连接路径；单源或不可达 → **低置信度**，不把"有响应"当"一致"。
- **relay 治理闭环**（P1）：relay 单独的成员证书 role、每成员/ASN/证书链配额、**预约证明**、带宽上限、排队、**撤销即时生效**；**relay 列表与出口目录分域**（一次发现不暴露两类角色关联）。
- **异常检测**（P2，弱化）：只留最小化/聚合/短保留指标；配额**按证书主体优先**，网络属性仅附加信号；自动隔离须短 TTL + 可审计恢复路径（避免变成成员活动遥测与误封源，尤其共享 NAT/relay 无法可靠归因）。

## 9. 冷启动 + NAT 可达（拆场景，去矛盾）

**场景 A：协调器失效、C2/带外可用**（可自动恢复）
1. 内置签名种子 DHT 节点不可达 → `RefreshFromBootstrap`（C2）拉 `TypeBootstrapBundle`，**验签 + 验证内含的 bootstrap 证书状态快照**。
2. 连 bootstrap（**过入站认证握手**，首次只允 bootstrap 所需操作），入私有 DHT（client 模式）。
3. NAT：Circuit Relay v2 预约（relay 经带外/DHT 冗余发现，过 §8 治理）+ DCUtR 打洞；失败走中继。
4. 按目标 region 的**当前 epoch capability** 派生 CID，`FindProviders` 取候选。
5. 认证 `NodeRecord` 协议向候选拉记录 → §4 四状态验证 + 连通性挑战。
6. 连接出网；回填 A3 缓存（含所用撤销 checkpoint 版本）。

**场景 B：所有带外渠道失效**（**不承诺自动冷启动**，P0 去矛盾）
- 只能靠**预先缓存**（A3，且在撤销陈旧上限内）/ **社交恢复** / **人工带外**取得新 bundle。手动 `-connect` 对普通成员**不是可运营恢复机制**，只作技术下限。内置种子须带**签名版本/过期/撤销/多发布渠道**。

## 10. 子切片顺序（每步 TDD + 红队）

| 子切片 | 内容 | 关键测试 / 红队 |
|---|---|---|
| **C1.0 契约冻结 + blocker 矩阵 spike** ✅**已完成** | ①**blocker 矩阵已实测 GO**（[c1-dht-compat.md](c1-dht-compat.md)：前缀隔离/RT-filter/gater 服务端强制/provider 单指针/PutValue 单值/pnet 兼容 六项成立；kad-dht v0.42.1 恰配 libp2p v0.48.0）②**契约已冻结**（§15：对象 schema+版本、protocol ID、两段入站认证、挑战绑主体、pnet 默认关、版本协商+降级拒绝） | 无 blocker 命中"停止假设" → **私有认证成员 DHT 可行**，进入 C1.1。探针跑完已移除、go.mod 复原（kad-dht 至 C1.1 再入） |
| **C1.1 离线根 + 在线受限 issuer + 成员证书** | 离线根签 issuer 子证书；协调器准入后用 issuer 签普通成员证书；出口/relay 证书须根/双签；客户端 issuer allowlist | 无效/过期/被撤销/越权(在线 issuer 签出口)证书 → 拒 |
| **C1.2 撤销 checkpoint + 陈旧兜底** | 根签 checkpoint（thisUpdate/nextUpdate/版本/陈旧上限）；带外+DHT 分发；离线超期 → 应急受限 scope | 撤销成员记录被拒；超陈旧上限 → 拒建普通隧道、仅应急；防 fail-open |
| **C1.3 私有认证 DHT 引导 + 入站门控** | node 挂 DHT + `/whatgate/kad` 前缀 + **ConnectionGater 成员握手**；server/client 模式；bootstrap bundle 供给；双轨接线 | 未认证连接不进路由表/无 RPC；无协调器时经认证 DHT 发现 |
| **C1.4 节点记录两层发现 + 四状态合并 + 拨号策略** | epoch capability CID → FindProviders → 认证 NodeRecord 拉取；四状态状态机 + equivocation 隔离 + 连通性挑战 + 地址 gater/拨号预算 | 伪出口/替换址/重放旧 generation/equivocation/SSRF 地址/无 recommended 补齐 → 拒或受限；合法短期记录 → 采信 |
| **C1.5 Sybil/eclipse/relay 治理** | 每主体配额 + 绑私钥挑战/blind token；多路径+独立性一致性；relay role 配额/预约证明/分域 | Sybil 灌入受限；eclipse/近邻占位检测；relay 耗尽被拒 |
| **（后续，默认关闭）rendezvous / 枚举加固 / 身份轮换** | 独立威胁模型 + 量化匿名度后再启用 | 单独隐私验收（**是否纳入 v1 待用户裁决**） |

## 11. 对手驱动验收

- **断协调器仅经认证 DHT 连通**（含 A1/A3/C2 全关）→ 仍能发现并出网。
- **未认证探测 / Sybil 灌入 / eclipse / k-近邻占位 / 伪出口 / SSRF 地址 / equivocation / 撤销陈旧** → 各独立量化红队用例，被拒或受限。
- **每子项定义"失败安全拒绝"** + 自动化故障注入。
- **双异网真机（NAT/CGNAT）**实测冷启动 + 打洞/中继成功率 + reservation 成功率（回环不算数）。

## 12. 上线门槛 / 风险 / 退出条件

- **CGNAT 容量门槛（P0，上线前置）**：可入站 DHT server/relay 的**最小数量、AS 分布、带宽、reservation 成功率、CGNAT 比例**达标才上线。**协调器可运营 DHT server/relay 作"受信容量底座"**，但明确标注**非去中心化证明**，须≥2 家独立运营者、AS 分散、可替换 bundle、可量化退出门槛；**达不到 → C1 只标弱冗余，不承诺协调器全失效后普遍可用**。
- **kad-dht × v0.48 × relay/pnet 未知坑** → C1.0 blocker 矩阵先行；不达标回退"协调器为主 + DHT 弱冗余"。
- **B1 pnet × C1 先决**（P1）：C1.0 前**固定**——启用 pnet 则全 bootstrap/relay/DHT 共享 swarm key（单点泄露=网络级准入放大）；不启用则依赖入站证书认证。**禁止"先叠加再看"**；pnet 只作传输层门槛，**不替代**证书/撤销/配额。
- **DHT 流量可指纹** → 与 B2 协同，单独 C1 不改数据面可识别性。
- **枚举残余风险**：已授权恶意成员可枚举其 scope——记录并量化，不归零。

## 13. 开放问题的评审裁决

| # | 问题 | 裁决（采纳 Codex） |
|---|---|---|
| 13.1 | region key/payload 加密 vs 可发现性 | **不做全体共享加密**；每 epoch/角色/region 派生 capability namespace + 认证入站 access control + 最小 payload；已授权成员枚举列为残余风险。 |
| 13.2 | 签发根 | **在线协调器根不可接受**；一期即**离线根 + 在线受限 issuer**；出口/relay 授权须根/双签；阈值/多签可后置。 |
| 13.3 | rendezvous 一期是否必须 | **否**；DHT 索引 + 认证记录拉取先独立验收；rendezvous 默认关闭、独立隐私模型后再启用。 |
| 13.4 | 短期 PeerID 轮换 | **v1 禁止**；只轮换地址/广告密钥；身份迁移作独立协议项目。 |
| 13.5 | 双轨信任合并 | 仅保守 scope 过滤**不足**；用 `authorized/eligible/recommended/reachable` 状态机，DHT 永不给 recommended，权威失联仅 pin/应急，UI/日志暴露来源与新鲜度。 |
| 13.6 | 跨模块契约后补 | **C1.0 前冻结** A0/C2/F5/B1/B2 接口契约 + 版本协商 + 降级拒绝。 |
| 13.7 | 高 CGNAT 容量 | 允许协调器运营受信容量底座，但要求多运营者/AS 分散/可替换/量化门槛；否则只标弱冗余。 |

## 14. Codex 评审记录（`/cx-plan`，2026-07-26）

Codex（只读对抗评审）提出约 19 条独立问题（~16×P0、~10×P1、1×P2）+ 对 7 个开放问题的取舍，**几乎全部采纳并折叠入上文**。最关键的架构性修正：

1. **前缀非访问控制 → 入站认证门控**（P0，§5）：本稿最大修正——原稿把"私有前缀 + 最终不采信"当私有，错。
2. **provider ≠ 签名记录层**（P0，§3）：拆两层 FindProviders→认证 NodeRecord 拉取；provider TTL 与记录 TTL 解耦 + 连通性挑战。
3. **离线根 + 在线受限 issuer**（P0，§4，覆盖原 13.2 倾向）：在线单签根是"任意 Sybil/出口授权"单点。
4. **撤销闭合**（P0，§4）：签名 checkpoint + 陈旧上限 + 超期仅应急，防 fail-open/全网误拒。
5. **四状态信任合并**（P0，§2/§4）：DHT 永不给 recommended；权威失联仅 pin/应急。
6. **k-近邻劫持 + 低熵 region 键**（P0，§2/§5）：改 epoch 化派生 capability namespace。
7. **拜占庭 equivocation + 单写者**（P0/P1，§4）：generation + payload-hash + 同 generation 异 hash 隔离；writer lease。
8. **DHT 地址拨号 SSRF**（P0，§4.5）：地址 gater + 拨号预算。
9. **冷启动前提矛盾**（P0，§9）：拆"C2 可用"与"全带外失效"两场景，后者不承诺自动恢复。
10. **CGNAT 容量门槛**（P0，§12）：设为上线前置；协调器容量底座非去中心化证明。
11. **PeerID 不可轮换 / rendezvous 移出 v1 核心**（P0，§6/§13.4）。
12. **pnet × C1 先决**（P1，§12）、**挑战禁用共享 HMAC**（P1，§8）、**一致性独立性定义**（P1，§8）、**relay 治理闭环**（P1，§8）、**C1.0 blocker 矩阵**（P1，§10）。

**范围冲突已裁决（2026-07-26）**：用户原选"v1 全量一期做完"（含 rendezvous + 枚举加固 + 身份轮换）；Codex P0/P1 建议把 **rendezvous 与身份轮换移出 v1 核心**（默认关闭、独立后续），理由是它们扩大半中心化在线情报面/与证书信任模型冲突，且不能弥补 DHT 信任缺陷。**用户已裁决采纳 Codex**：rendezvous 与 PeerID 轮换移出 v1 核心（§6、§10 末、§13.4）；**枚举面加固保留在 v1 核心**（§5/§7）。v1 = 认证 DHT 索引 + 认证记录拉取 + 四状态合并 + Sybil/eclipse/relay 治理 + 枚举加固。

## 15. C1.0 契约冻结（2026-07-26，据 blocker 矩阵 GO 后冻结）

> **前置状态**：blocker 矩阵**已实测 GO**（[c1-dht-compat.md](c1-dht-compat.md)：前缀隔离/RT-filter/gater 强制/provider 单指针/PutValue 单值/pnet 兼容 六项成立）。以下把 C1.1+ 依赖的跨模块契约冻结为**带版本 + 降级拒绝**的确定形态，避免后补返工（评审 13.6）。**格式冻结，字段可加不可改语义**。

### 15.1 签名对象（复用 A0 `discovery.Signed`，新增 type + 每对象 `v` 字段）

所有对象走 `discovery.Signed` 信封（域分隔 `canonical`、单调序号、过期、`RevocationEpoch`、公钥钉扎）。payload 内首字段一律 `"v": 1`（对象格式版本）；消费者**拒绝 `v` 高于自身支持上限或低于 `minAcceptable`**。

- `TypeMemberCert`（成员证书）：`{v, subject(PeerID), issuer, roles[], capsHash, notBefore, notAfter, serial, revEpoch}`。`roles ⊆ {member, exit, relay}`；**`exit`/`relay` 角色的证书须由离线根或双签 issuer 签发**（在线受限 issuer 只能签 `member`）。
- `TypeNodeRecord`（出口/中继记录）：`{v, subject(PeerID), roles[], addrs[], region, epoch, generation, notAfter(分钟级), cert(内嵌或引用 TypeMemberCert)}`。**主键=generation 严格递增**（反回滚），签名时间仅作界。
- `TypeRevocationCheckpoint`（撤销 checkpoint）：`{v, issuerRoot, thisUpdate, nextUpdate, version(单调), maxStaleness, revoked[](subject+revEpoch), rootRotation?}`。**离线超 `maxStaleness` → 仅应急受限 scope**，不建普通隧道。
- `TypeBootstrapBundle`（C2 扩展，替代裸 `BootstrapList`）：`{v, endpoints[](协调器), dhtSeeds[](bootstrap PeerID+addrs), certStatusSnapshot(最小), serial, notAfter}`。C2 `RefreshFromBootstrap` 消费时**验签 + 验内含 certStatusSnapshot**。

### 15.2 protocol ID（semver，`/whatgate/<name>/<major.minor>`）

- **DHT**：`dht.ProtocolPrefix("/whatgate/kad")` → `/whatgate/kad/1.0.0`（与公有 IPFS DHT 隔离，实测成立）。
- **成员认证握手**：`/whatgate/member-auth/1.0.0`——连接建立后（Noise 已认证 PeerID）跑此协议交换 `TypeMemberCert` + 应答**挑战**（§15.4）。**未通过 → 断连、不进路由表、不答任何 DHT/记录 RPC。**
- **节点记录拉取**：`/whatgate/noderecord/1.0.0`——查询者对 `FindProviders` 得到的候选 PeerID **直接**发起此认证 stream 拉取其 `TypeNodeRecord`（**不经 provider payload、不经 `PutValue`**，实测证实二者不可承载多值/带 payload）。含**连通性挑战 nonce**（防挂旧址）。
- **撤销 checkpoint 分发**：`/whatgate/revocation/1.0.0`（DHT 冗余）+ C2 带外。

### 15.3 DHT 入站认证（两段，实测约束驱动）

blocker 矩阵证实 gater 在 `InterceptSecured` 只拿到 **Noise 认证过的 PeerID**（拿不到证书），故冻结为**两段**：
1. **粗准入（PeerID 粒度）**：`ConnectionGater` + `dht.RoutingTableFilter` 用**本地已知成员 PeerID 集合**（来自协调器目录/带外/已验证记录缓存）过滤——非集合内 PeerID **服务端强制不保留连接、不进路由表**（实测 `Connectedness=NotConnected` + RT 无条目；**不依赖"远端连不上"**）。
2. **证书校验（连接后）**：`/whatgate/member-auth/1.0.0` 校验 `TypeMemberCert` 链 + 未撤销 + 挑战。**两段都过才服务 RPC/记录拉取。**

> 首次冷启动的成员 PeerID 集合来自 `TypeBootstrapBundle.certStatusSnapshot` 与 bootstrap 节点的 member-auth，解冷启动循环（评审 P0）。

### 15.4 挑战绑定（禁用共享 HMAC，评审 P1）

挑战 = **服务端 nonce + 请求方成员私钥签名**（沿用 `internal/authn` 思路，绑 PeerID），或 issuer 发放的**不可转让限额 blind token**。**禁止全体共享 HMAC**（泄露一成员即可离线伪造）。与安全评审 F5 合并设计。

### 15.5 pnet 模式（与 B1 决策合并）

blocker 矩阵证实 **DHT 在 pnet 下可用**（TCP+WS）。但**当前 B1 暂缓 → C1 默认不启用 pnet**，成员性靠 §15.3 的 DHT 入站证书认证。若未来启用 pnet：作**传输层门槛**，**不替代**证书/撤销/配额；启用即全 bootstrap/relay/DHT 共享 swarm key（单点泄露=网络级准入放大，须接受）。**禁止"先叠加再看"**——切换须重跑攻击矩阵。

### 15.6 版本协商 + 降级拒绝

- **对象**：`v` + `minAcceptable`；低于下限或高于上限 → 拒。
- **协议**：libp2p multistream 协商 `/whatgate/*/<major.minor>`；**major 不匹配拒绝**，minor 向后兼容。
- **降级攻击**：消费者记录已见的最高 `major`，拒绝被诱导降级到更低 major（类 A0 反回滚 floor 思路）。

> **C1.0 完成判据**：blocker 矩阵 GO（✅）+ 本节契约冻结（✅）。下一步 C1.1（离线根 + 在线受限 issuer + 成员证书），正式引入 `go-libp2p-kad-dht@v0.42.1` 到 go.mod。
