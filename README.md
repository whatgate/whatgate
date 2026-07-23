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
- **技术栈**：Go + [libp2p](https://libp2p.io/)。
- **分发**：编译为单个静态二进制，终端用户下载即用，无需安装任何运行时。

## 快速开始

### 构建

```bash
go build -o bin/ ./...
```

产出 `bin/coordinator` 与 `bin/node`（Windows 为 `.exe`）。终端用户无需装 Go，直接用预编译二进制。

全局 TUN 模式（可选，体积更大、需管理员权限运行）：

```bash
go build -tags tun -o bin/node-tun ./cmd/node
```

（运行方式与平台前置见 [docs/tun-and-mobile.md](docs/tun-and-mobile.md)。）

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
# 协调器（兼跑中继），播种邀请码
coordinator -addr :8080 -invite welcome

# 出口节点：加入并注册为某地区(如 JP)出口
node -coordinator http://<host>:8080 -invite welcome -exit -region JP

# 客户端：加入并按地区自动发现出口
node -coordinator http://<host>:8080 -invite welcome -to JP -socks 127.0.0.1:1080

curl --socks5-hostname 127.0.0.1:1080 https://api.ipify.org
```

### 出口保护（ExitGuard）

作为出口时可加保护策略，抵御被陌生人滥用：

```bash
# 只给自己小网/已认证小网当出口；封禁高危端口；限并发
node -coordinator http://host:8080 -invite welcome -exit -region JP \
     -group myfriends \
     -exit-scope conservative \
     -block-domains evil.example.com \
     -max-conns 50
```

- `-exit-scope conservative`：只服务信任圈内的请求者（陌生人被拒）
- SMTP 端口（25/465/587）默认封禁；`-block-ports` 追加
- `-block-domains`：目标域名黑名单
- `-max-conns`：最大并发连接数

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

后续增强项（安全加固、UDP、持久化、UI、移动端落地等）见 **[docs/backlog.md](docs/backlog.md)**。

> 注：真实跨 NAT 的打洞与 AutoRelay 自动预约需在两台异网机器上实测；本地回环已验证隧道与中继数据路径本身。

## 许可

TBD
