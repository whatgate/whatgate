# 开发、构建与发布

## 1. 技术栈

Core：Go、go-libp2p、WhatGate tunnel / datagram protocol、HTTP/JSON Coordinator。

Desktop：C#、.NET、Avalonia、XAML、CommunityToolkit.Mvvm。

## 2. 本地构建

Core：

    go build -o bin/ ./...

可选 TUN：

    go build -tags tun -o bin/whatgate-tun ./cmd/whatgate

Desktop：

    dotnet build desktop/WhatGate.Desktop/WhatGate.Desktop.csproj

## 3. 测试

    go test ./...
    go vet ./...

详细测试矩阵见 [testing.md](testing.md)。

重点包：

- pkg/protocol：wire 编解码
- internal/tunnel：隧道、授权、超时
- internal/proxy：SOCKS5 TCP / UDP
- internal/node：真实 libp2p 节点与 relay
- internal/coordinator：join/register/directory/group/trust/reputation
- internal/trust：group / tier / reputation
- internal/routing：region / trust / latency / load
- internal/exit：ExitGuard / SSRF / rate / bandwidth

## 4. 真实环境验证

单元测试通过不等于生产网络已经验证。

P2P 至少测试不同网络、双 NAT、无入站 IPv4，以及 hole punching 失败时的 relay fallback。

Desktop 至少测试 Windows x64 / ARM64、Linux x64 / ARM64、macOS x64 / arm64，以及安装、启动、停止、重启核心。

TUN 至少测试管理员/root、默认路由、核心自身流量绕行、IPv4 / IPv6 和 DNS 行为。

ExitGuard 至少测试私有 IP、DNS rebinding、port/domain block、per-requester limits 和 bandwidth breaker。

Control plane 至少测试多端点 failover、签名验证失败、serial rollback、cache fallback 和 bootstrap recovery。

## 5. Release

核心：

    scripts/build-release.sh v0.3.0

桌面：

    ./scripts/build-desktop.ps1 -Version v0.3.0

Release workflow：

    .github/workflows/release.yml

正式发布应在 Windows、Linux、macOS 原生环境构建对应架构包。

## 6. PR / 文档同步

1. 一个 PR 聚焦一个主题。
2. 新功能更新 docs/features.md。
3. CLI flag 变化更新 docs/configuration.md。
4. 架构边界变化更新 docs/architecture.md。
5. 安全行为变化检查 docs/security-review.md。
6. 实验能力明确标注“实验 / 待真实验证”。

## 7. 目录职责

    cmd/         可执行程序入口
    desktop/     桌面 UI 与打包
    docs/        用户 / 运维 / 架构 / 安全文档
    internal/    核心实现
    pkg/         可复用 protocol 包
    scripts/     构建与 E2E 辅助脚本
