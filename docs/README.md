# WhatGate 文档中心

文档按“用户 → 管理员 → 贡献者 → 安全 / 研究”分层，避免把正式能力、部署注意事项和未来设计混在一起。

## 阅读路径

| 读者 | 文档 | 用途 |
|---|---|---|
| 普通用户 | [项目 README](../README.md) | 下载、创建/加入、代理使用 |
| 网络管理员 | [configuration.md](configuration.md) | Coordinator、出口策略、TLS、持久化 |
| 贡献者 | [development.md](development.md) | 构建、测试、桌面端、Release |
| 架构阅读 | [architecture.md](architecture.md) | 控制面、数据面、模块和时序 |
| 安全审阅 | [security-review.md](security-review.md) | 历史问题、修复状态、剩余风险 |
| 抗封锁研究 | [anti-censorship.md](anti-censorship.md) | Tier A/B/C 与敌手模型 |

## 正式能力

- [features.md](features.md) — 当前代码实现的功能清单和能力边界
- [configuration.md](configuration.md) — whatgate / coordinator 参数
- [architecture.md](architecture.md) — 运行时架构与数据流
- [dns.md](dns.md) — DNS 解析位置与防泄漏
- [development.md](development.md) — 开发、构建、发布
- [testing.md](testing.md) — 自动化与真实环境测试

## 安全与运营

- [security-review.md](security-review.md)
- [anti-censorship.md](anti-censorship.md)
- [backlog.md](backlog.md)

## 实验与设计稿

- [c1-decentralized-discovery.md](c1-decentralized-discovery.md)
- [c1-dht-compat.md](c1-dht-compat.md)
- [pnet-compat.md](pnet-compat.md)
- [tun-and-mobile.md](tun-and-mobile.md)

## 文档维护规则

1. 代码事实优先：功能是否“已实现”，以当前 cmd / internal / desktop 和测试为准。
2. 实现与设计分开：设计稿可以讨论未来能力，但 README 不把它们写成现成功能。
3. 安全结论带边界：区分已实现的控制和未完成的真实网络验证。
4. README 只保留稳定入口；详细操作放到专题文档。
5. 新增 CLI flag 后，同步检查 configuration.md。
