# WhatGate

[![Release](https://img.shields.io/github/v/release/whatgate/whatgate)](https://github.com/whatgate/whatgate/releases/latest)
[![Release build](https://github.com/whatgate/whatgate/actions/workflows/release.yml/badge.svg)](https://github.com/whatgate/whatgate/actions/workflows/release.yml)
[![License](https://img.shields.io/github/license/whatgate/whatgate)](LICENSE)

**面向信任圈的跨平台 P2P 网络客户端。**

WhatGate 把在线设备组成一个由成员邀请、信任圈和可选共享出口构成的 P2P 网络。客户端按目标地区发现出口，把浏览器或应用的代理流量通过加密的 libp2p 隧道送到远程出口；协调器只负责准入、节点发现和控制元数据，不承载代理业务流量。

> 桌面客户端是普通用户的主要入口，不需要手动操作本地 Web 控制台，也不需要安装 Go 或 .NET。

## 当前能力

| 能力 | 状态 | 说明 |
|---|---|---|
| 跨平台桌面客户端 | ✅ | Windows / Linux / macOS，Avalonia + XAML |
| 创建 / 加入网络 | ✅ | 首位成员自举；后续通过邀请码加入 |
| 本地 SOCKS5 | ✅ | TCP CONNECT，并支持 UDP ASSOCIATE |
| P2P 加密隧道 | ✅ | libp2p，直连优先，必要时可走 Circuit Relay v2 |
| 地区选路 | ✅ | 地区过滤，并结合信任、延迟、负载排序 |
| 信任圈 | ✅ | 小网成员、跨组认可、conservative / open |
| 声誉系统 | ✅ | 成员 / 小网声誉、事件反馈、衰减、持久化 |
| ExitGuard | ✅ | 信任、端口/域名/IP/CIDR、并发、速率、带宽熔断、SSRF |
| DNS 出口解析 | ✅ | 主机名由出口侧解析；TCP 可指定 DNS 服务器 |
| 威胁情报 | ✅ | threat feed 定期更新恶意域名黑名单 |
| 审计与指标 | ✅ | JSON Lines 审计；本地 JSON metrics |
| Coordinator 多端点 | ✅ | failover、已验证目录缓存、签名 bootstrap 自愈 |
| 控制面响应签名 | ✅ | directory / relay / bootstrap + pinning + anti-rollback |
| 全局 TUN | 🧪 | 代码已实现，需要平台与真实网络验证 |
| 私有认证 DHT | 🧪 | Tier C 实验能力，真实异网验证仍是前提 |
| 主动探测抗性 / 混淆 | 🗺️ | 后续路线图，不等同于现有 TLS / 多端点能力 |

完整盘点见 docs/features.md。

## 下载

正式安装包以 GitHub Releases 为准：

https://github.com/whatgate/whatgate/releases

Windows：解压后运行 Install-WhatGate.cmd。
Linux：解压后运行 install.sh。
macOS：将 WhatGate.app 放入“应用程序”。

## 第一次使用

### 创建新网络

1. 打开 WhatGate，选择“创建我的网络”。
2. 选择协调端口与目标地区，默认使用“仅信任圈”。
3. 点击“创建并启动网络”。
4. 客户端启动本地协调器，并把当前设备登记为首位管理员。
5. 生成成员邀请码，把协调器地址和邀请码交给可信成员。

首位成员自举只在网络没有成员时可用；之后加入必须使用邀请码。

### 加入已有网络

1. 选择“加入已有网络”。
2. 填写管理员提供的协调器地址和邀请码。
3. 选择目标地区与信任范围。
4. 启动连接。
5. 把浏览器或应用的 SOCKS5 代理设为 127.0.0.1:1080。

互联网部署时，协调器应使用 HTTPS；明文 HTTP 只适合可信局域网控制面。

## 使用代理

默认本地 SOCKS5 地址：

127.0.0.1:1080

测试远端出口：

    curl --socks5-hostname 127.0.0.1:1080 https://api.ipify.org

使用域名时应优先选择“远端解析 DNS”的 SOCKS5 模式，详见 docs/dns.md。

## 共享出口

节点可以主动成为出口。共享出口默认关闭，开启后其他成员的目标连接会从该设备出网。

ExitGuard 可控制：

- 谁能使用出口：trust scope / reputation
- 哪些端口、域名、IP/CIDR 可以访问
- 总并发、单请求方并发与建连速率
- 单请求方带宽与熔断
- 私有、环回、链路本地和云 metadata 目标
- 威胁情报黑名单
- 审计日志和运行指标

详见 docs/configuration.md。

## 命令行快速上手

协调器：

    coordinator -addr :8080 -invite welcome -state ./state.json -signing-key ./signing.key

出口：

    whatgate -exit -region JP

按地区发现出口：

    whatgate -coordinator https://<host>:8080       -coordinator-key <协调器公钥>       -invite <邀请码>       -to JP       -trust-scope conservative       -socks 127.0.0.1:1080

手动直连出口：

    whatgate -exit
    whatgate -connect <出口多地址> -socks 127.0.0.1:1080

完整参数见 docs/configuration.md。

## 项目结构

    cmd/coordinator/          Coordinator 命令行入口
    cmd/whatgate/             节点命令行入口
    desktop/                  Avalonia 桌面客户端

    internal/coordinator/     准入、目录、信任/声誉控制面
    internal/node/            libp2p host、NAT、打洞、中继、成员门控
    internal/tunnel/          客户端/出口隧道
    internal/proxy/           SOCKS5 / UDP 入口
    internal/routing/         地区、信任、延迟、负载选路
    internal/trust/           信任圈与声誉
    internal/exit/            ExitGuard 与 SSRF 防护
    internal/discovery/       控制面签名对象
    internal/membership/      成员凭据、角色、撤销
    internal/tun/             全局 TUN
    internal/relay/           Circuit Relay v2
    internal/webui/           本地状态控制接口
    internal/metrics/         运行指标
    internal/audit/           审计日志
    internal/config/          JSON 配置覆盖
    pkg/protocol/             隧道 / datagram wire 协议

## 文档导航

- [文档中心](docs/README.md)
- [功能总览](docs/features.md)
- [架构与数据流](docs/architecture.md)
- [参数与配置](docs/configuration.md)
- [开发、构建与发布](docs/development.md)
- [测试与真实环境验证](docs/testing.md)
- [DNS 策略](docs/dns.md)
- [安全评审](docs/security-review.md)
- [抗封锁设计](docs/anti-censorship.md)
- [路线图 / 待办](docs/backlog.md)
- [去中心化发现设计](docs/c1-decentralized-discovery.md)
- [DHT 可行性](docs/c1-dht-compat.md)
- [pnet 兼容性](docs/pnet-compat.md)
- [TUN / 移动端设计](docs/tun-and-mobile.md)
- [桌面客户端开发](desktop/README.md)

## 开发状态

核心 M1–M6 链路已经实现；“代码已实现”与“所有平台/公网环境已经验证”不是同一个状态。

需要重点区分：

- 跨 NAT 打洞、中继、TUN 默认路由等能力仍需要真实环境验证。
- 私有 DHT 属于实验能力。
- 多协调器、TLS、签名 bootstrap 解决的是控制面可用性和真实性，不等于完整的流量混淆或主动探测抗性。

许可：[MIT](LICENSE) © 2026 WhatGate
