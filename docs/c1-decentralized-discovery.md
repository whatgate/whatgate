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
| **rendezvous / 枚举加固 / 身份轮换** | **移出 v1 核心，待用户裁决**（评审 P0/P1，与"全量"选择冲突） | Codex 强烈建议：v1 冻结在"**认证 DHT provider 索引 + 认证记录拉取**"通过验收即止；rendezvous 作**默认关闭的独立后续能力**（它扩大半中心化在线情报面）。**这与用户"v1 全量一期"的选择冲突，需用户拍板**（见 §14 末与报告）。 |
| **跨模块契约** | **C1.0 前冻结**（评审 P0） | 证书/撤销 checkpoint 格式、bootstrap 信任包、DHT 入站认证、挑战绑定主体、pnet 模式、B2 对 DHT/relay 流量的覆盖——**先冻结带版本协商 + 降级拒绝**，否则后补必返工。 |

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

## 6. rendezvous（移出 v1 核心，待裁决）

- Codex P0/P1：rendezvous 引入**半中心化在线情报库**，且**不能弥补 DHT 信任缺陷**。**建议 v1 不做**，作**默认关闭的独立后续能力**：需成员认证、每成员/角色 **blind capability**、注册/发现权限分离、固定返回上限、抗关联 padding、运营者视为可观察对手；**完成独立威胁模型 + 量化匿名度前不接生产发现**。
- **与用户"全量"选择冲突，见 §14 末**。

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
| **C1.0 契约冻结 + blocker 矩阵 spike** | ①冻结跨模块契约（证书/撤销 checkpoint/bootstrap bundle/DHT 入站认证/挑战绑主体/**pnet 模式**/B2 覆盖，带版本协商+降级拒绝）②**blocker 矩阵**探 v0.48：**入站 DHT 鉴权(ConnectionGater)**、私有 validator、**多值索引 vs provider 两层**、路由表污染防护、resource manager 限额、**撤销陈旧处理**、provider 刷新。产出 `docs/c1-dht-compat.md` | 任一 blocker 在 v0.48 组合中**不可实现 → 停止"私有成员 DHT"假设**，改显式认证 discovery 服务/自定义协议。不以"应用后验"掩盖缺口 |
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

**唯一需用户裁决的冲突**：用户选了"v1 全量一期做完"（含 rendezvous + 枚举加固 + 身份轮换）；Codex P0/P1 强烈建议把 **rendezvous 与身份轮换移出 v1 核心**（默认关闭、独立后续），理由是它们扩大半中心化在线情报面/与证书信任模型冲突，且不能弥补 DHT 信任缺陷。**本稿据 Codex 暂将其列为"后续、默认关闭"（§10 末），最终纳入范围以用户裁决为准。**
