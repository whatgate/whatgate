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

### 测试

```bash
go test ./...
```

## 目录结构

```
cmd/coordinator   协调服务器入口（兼跑 Circuit Relay）
cmd/node          客户端核心入口（既是客户端也是出口）
internal/proxy    本地 SOCKS5 入口
internal/tunnel   隧道两端（出/入），解耦具体传输
internal/node     libp2p 接入：host、隧道、NAT 穿透、中继
internal/relay    Circuit Relay v2 中继服务
internal/coordinator  节点目录、邀请准入、HTTP 控制面
internal/routing  选路引擎（按地区，后续加测速/信任）
pkg/protocol      隧道 wire 协议（目标地址编解码）
```

## 开发状态

- **M1** 骨架：本地 SOCKS5 + 两节点 libp2p 直连出网 ✅
- **M2** 组网：邀请准入 + 节点目录/发现 + NAT 穿透 + Circuit Relay 兜底 ✅
- **M3** 小网与信任：小网/信任图/两级声誉/信任范围过滤/首启动向导 ⬜
- **M4** 选路：地区 + 延迟/负载/信任度自动选最优 ⬜
- **M5** 出口治理：ExitGuard 策略、限额熔断、可追溯、威胁情报 ⬜
- **M6** 扩展：TUN 全局模式 → 移动端 ⬜

> 注：真实跨 NAT 的打洞与 AutoRelay 自动预约需在两台异网机器上实测；本地回环已验证隧道与中继数据路径本身。

## 许可

TBD
