# WhatGate 测试指南

本文档说明如何验证 WhatGate 的各项功能：从单元测试，到多进程端到端出网，到各里程碑的关键行为（信任范围、出口保护、TUN）。

- 架构见 [architecture.md](architecture.md)
- TUN / 移动端见 [tun-and-mobile.md](tun-and-mobile.md)

## 0. 前置：构建

```bash
go build -o bin/ ./...
```

产出 `bin/coordinator` 与 `bin/whatgate`（Windows 为 `.exe`）。下文命令用 `./bin/whatgate`；Windows 用 `bin\whatgate.exe`。

## 1. 单元测试

一条命令跑全部单元/集成测试：

```bash
go test ./...
go vet ./...
```

全绿即通过。各包覆盖：

| 包 | 测试覆盖 |
|---|---|
| `pkg/protocol` | 目标地址长度前缀编解码往返 |
| `internal/tunnel` | 出口 `ServeExit`（读目标/授权/拨号/转发）、客户端 `ClientDialer`（用 `net.Pipe` + echo 回环） |
| `internal/proxy` | SOCKS5 入口握手 / 域名解析 / 双向转发（独立 SOCKS5 客户端对打） |
| `internal/node` | **两个真实 libp2p 节点**隧道；**中继隧道**（出口预约、客户端仅凭 circuit 地址连通） |
| `internal/coordinator` | 目录注册/过期/注销、邀请准入与可追溯、HTTP `/join·/register·/directory·/relay·/group·/trust`、节点客户端 |
| `internal/trust` | 信任图/背书/信任层级、信任范围 Scope、两级声誉 |
| `internal/routing` | 按地区筛选、信任范围过滤、`RankExits`（信任→延迟→负载排序） |
| `internal/exit` | ExitGuard 授权（拒陌生/端口/域名黑名单/并发限额） |

单独跑某包并看详情：

```bash
go test ./internal/trust/ -v
```

## 2. 端到端出网测试（多进程）

真正证明"流量经隧道从出口出网"。需要几个终端窗口。

> **验证核心**：客户端经本地 SOCKS5 访问一个回显 IP 的服务，返回的应是**出口**的公网 IP。同一台机器上两个 IP 会相同（正常）；要看出"换地区"的效果，需把出口放在**另一台不同网络的机器**上，届时隧道那条会显示出口的 IP。

### 2.1 方式 A —— 手动直连（M1，无需协调器）

```bash
# 终端 1：出口节点。复制它打印的某个 /p2p/ 多地址
./bin/whatgate -exit

# 终端 2：客户端，经该出口建隧道
./bin/whatgate -connect <出口多地址> -socks 127.0.0.1:1080

# 终端 3：验证
curl --socks5-hostname 127.0.0.1:1080 https://api.ipify.org
```

### 2.2 方式 B —— 经协调器发现（M2）

```bash
# 终端 1：协调器（兼跑中继），播种邀请码
./bin/coordinator -addr :8080 -invite welcome

# 终端 2：出口，加入并注册为 JP 区出口
./bin/whatgate -coordinator http://127.0.0.1:8080 -invite welcome -exit -region JP

# 终端 3：客户端，按地区自动发现出口
./bin/whatgate -coordinator http://127.0.0.1:8080 -invite welcome \
           -to JP -trust-scope open -socks 127.0.0.1:1080

# 终端 4：验证
curl --socks5-hostname 127.0.0.1:1080 https://api.ipify.org
```

客户端会打印 `selected exit ... (trust: ..., latency: ...ms, load: ...)`，体现 M4 的选路。

### 2.3 信任范围（M3）

客户端 `-trust-scope conservative` 时只会用信任圈内的出口：

- **陌生出口 + 保守** → 客户端打印 `no exit in region "JP" within conservative trust scope`（拒绝）。
- **同小网 + 保守** → 让出口和客户端都加 `-group fam`，即可选中并出网。
- **陌生出口 + 开放** → 可用。

例（同小网放行）：

```bash
# 出口（首个加入 fam 者设口令）
./bin/whatgate -coordinator http://127.0.0.1:8080 -invite welcome -exit -region JP -group fam -group-secret famkey
# 客户端（保守，但用同一口令同属 fam）
./bin/whatgate -coordinator http://127.0.0.1:8080 -invite welcome -to JP -trust-scope conservative -group fam -group-secret famkey -socks 127.0.0.1:1080
```

### 2.4 出口保护 ExitGuard（M5）

给出口加保护策略，观察它**拒绝**不该服务的请求：

```bash
# 出口：只服务信任圈内、封禁某域名、限并发
./bin/whatgate -coordinator http://127.0.0.1:8080 -invite welcome -exit -region JP \
           -exit-scope conservative -block-domains api.ipify.org -max-conns 50
```

- **陌生请求者**（客户端与出口无小网/背书关系）→ 隧道被拒，`curl` 返回空/失败。
- **同小网**（双方都 `-group fam`）→ 放行。
- **命中域名黑名单**（curl `https://api.ipify.org`）→ 拒；换未命中域名（如 `https://ifconfig.me`）→ 放行。
- SMTP 端口（25/465/587）默认封禁。

### 2.5 中继兜底（M2）

打洞失败时走中继的**数据路径**由 `internal/node` 的 `TestTunnelOverCircuitRelay` 覆盖（`go test ./internal/node/`）。真实跨 NAT 的自动打洞/中继预约需两台异网机器实测。

## 3. 一键回归脚本（Windows PowerShell）

仓库 `scripts/` 下有验证过的端到端脚本（自动起进程、curl、清理）：

```powershell
# 信任范围三场景：保守拒陌生 / 保守放同组 / 开放放陌生
pwsh scripts/e2e-trust-scope.ps1

# ExitGuard 四场景：拒陌生 / 放同组 / 域名黑名单命中拒 / 未命中放
pwsh scripts/e2e-exit-guard.ps1

# 声誉闭环：滥用被扣分 -> 连允许域名也被拒；全新节点畅通（对照）
pwsh scripts/e2e-reputation.ps1
```

脚本会在开头杀掉遗留的 `whatgate`/`coordinator` 进程，并用回环端口跑完整流程，最后打印 `OVERALL: SUCCESS`。需先 `go build -o bin/ ./...`。

## 4. TUN 全局模式（M6，需管理员）

```bash
go build -tags tun -o bin/whatgate-tun ./cmd/whatgate
```

运行需管理员/root 权限、Windows 需 `wintun.dll`，并手动配置路由（含把出口/协调器连接排除出 TUN，避免回环）。详细步骤见 [tun-and-mobile.md](tun-and-mobile.md)。**不要在生产/日常机器上随意开启**——它会接管整机流量。

## 5. 故障排查

| 现象 | 原因 / 处理 |
|---|---|
| 客户端选中一个"莫名的"出口 | 有**遗留进程**没退干净、污染了目录。先 `Get-Process whatgate,coordinator \| Stop-Process -Force`（Windows）或 `pkill -f 'bin/whatgate'`（*nix） |
| 协调器打印了 listening 却连不上 | 端口被占。coordinator 的 "listening" 打印在真正 bind 之前——先确认端口空闲、无残留协调器 |
| `curl` 经隧道返回空 | 可能是 ExitGuard 拒绝（预期行为），或出口没在跑；看客户端/出口日志 |
| 换机器后隧道 IP 仍相同 | 两端在同一网络/机器。真实解锁需出口在不同地区的机器 |
| TUN 起不来 | 缺管理员权限或 `wintun.dll`；引擎会 fatal 退出并打印原因 |
