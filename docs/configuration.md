# WhatGate 配置参考

配置分成两层：whatgate 节点客户端/出口，以及 coordinator 网络控制面/Relay。

两者都支持 config JSON，配置文件的键就是 flag 名。

优先级：**命令行 > 配置文件 > 默认值**。

未知配置键会报错。

## 1. whatgate

### 基础

| 参数 | 默认值 | 作用 |
|---|---|---|
| -version | false | 打印版本 |
| -config | 空 | JSON 配置 |
| -identity | 空 | 持久化节点私钥 |
| -log-format | text | text / json |
| -listen | /ip4/0.0.0.0/tcp/0 | libp2p 监听地址 |
| -connect | 空 | 手动连接出口 multiaddr |
| -socks | 127.0.0.1:1080 | 本地 SOCKS5 |
| -exit | false | 开启共享出口 |

### Coordinator / 成员

| 参数 | 作用 |
|---|---|
| -coordinator | 一个或多个协调器 URL |
| -coordinator-key | pin 控制面签名公钥 |
| -coordinator-cache | 最后一次已验证目录缓存 |
| -bootstrap-url | 带外签名 bootstrap URL |
| -invite | 加入网络的邀请码 |
| -bootstrap-founder | 首位成员一次性自举 |
| -member-cert | Tier C 成员凭据路径 |
| -dht | 启用实验性私有 DHT |
| -root-key | Tier C 离线 root 公钥 |
| -dht-epoch | DHT capability epoch |

### 地区 / 信任 / 选路

| 参数 | 作用 |
|---|---|
| -region | 本节点作为出口的地区标签 |
| -to | 客户端希望选择的出口地区 |
| -group | 小网 group ID |
| -group-secret | group 加入口令 |
| -endorse | fromGroup:toGroup 认可 |
| -trust-scope | conservative / open |
| -rank-trust-weight | trust 加权权重 |
| -rank-latency-weight | latency 加权权重 |
| -rank-load-weight | load 加权权重 |
| -latency-ewma-alpha | 延迟 EWMA 平滑系数 |

### ExitGuard

| 参数 | 作用 |
|---|---|
| -exit-scope | conservative / open |
| -block-ports | 额外端口黑名单 |
| -block-domains | 域名 / IP / CIDR 黑名单 |
| -max-conns | 总并发上限 |
| -max-conns-per-requester | 单请求方并发上限 |
| -requester-rate | 单请求方新连接速率 |
| -requester-burst | 新连接 burst |
| -requester-bandwidth | 单请求方持续吞吐上限 |
| -requester-bandwidth-burst | 带宽 burst |
| -min-reputation | 最低请求方声誉 |
| -allow-private-targets | 允许访问私网/环回/链路本地 |
| -dns-server | 出口 TCP DNS resolver |
| -audit-log | JSON Lines 审计路径 |
| -threat-feed | 恶意域名 URL / 文件 |
| -threat-feed-interval | 威胁源刷新周期 |

### 观测 / Web / TUN

| 参数 | 作用 |
|---|---|
| -metrics-addr | JSON metrics 地址 |
| -web | 本地状态 dashboard |
| -tun | 全局 TUN |
| -tun-device | TUN 名称 |
| -tun-mtu | TUN MTU |
| -tun-auto-route | 自动分配地址 / 接管默认路由 |
| -tun-addr | TUN 地址 |
| -tun-gateway | 物理默认网关 |

## 2. coordinator

| 参数 | 默认 | 作用 |
|---|---|---|
| -addr | :8080 | HTTP(S) 监听 |
| -invite | welcome | 启动时预置邀请码 |
| -uses | 100 | 预置邀请码使用次数 |
| -bootstrap-first-member | false | 允许一次首位成员自举 |
| -issuer | founder | 预置邀请码 issuer |
| -ttl | 60s | 目录条目 TTL |
| -tls-cert | 空 | TLS 证书 |
| -tls-key | 空 | TLS 私钥 |
| -signing-key | 空 | 控制面签名私钥 |
| -state | 空 | admissions/groups/reputation 持久化 |
| -rate-limit | 0 | 变更 API 每 IP 速率 |
| -rate-burst | 20 | 变更 API burst |
| -sybil-window | 1h | Sybil 统计窗口 |
| -sybil-max-identities | 0 | 每 IP distinct PeerID 上限 |
| -trusted-proxies | 空 | 可信反代/CDN IP/CIDR |
| -emit-bootstrap | 空 | 生成签名 bootstrap |
| -bootstrap-serial | 0 | bootstrap 单调 serial |
| -bootstrap-ttl | 30d | bootstrap 有效期 |
| -relay-listen | /ip4/0.0.0.0/tcp/0 | Relay 监听 |
| -relay-circuit-duration | 0 | 单 circuit 时长 |
| -relay-circuit-data | 0 | 单 circuit 数据量 |
| -relay-max-reservations | 0 | reservation 总量 |
| -relay-max-circuits-per-peer | 0 | 单 peer circuit 数 |
| -relay-max-reservations-per-ip | 0 | 单 IP reservation 数 |
| -relay-reservation-ttl | 0 | reservation 生命周期 |
| -reputation-decay | 1 | 声誉衰减步长 |
| -reputation-decay-interval | 1h | 声誉衰减周期 |

## 3. 推荐部署基线

互联网 Coordinator 至少考虑：

    coordinator       -addr :8080       -tls-cert ./fullchain.pem       -tls-key ./privkey.pem       -signing-key ./signing.key       -state ./state.json

客户端同时 pin：

    whatgate       -coordinator https://example.com:8080       -coordinator-key <public-key>

反向代理 / CDN 前置时，同时设置 trusted-proxies，否则 rate limit / Sybil detection 会按代理地址聚合。

公网共享出口建议保持 conservative exit scope、关闭 allow-private-targets，并根据出口带宽设置 max-conns 与 requester-bandwidth。

## 4. 配置文件安全

配置文件可以包含 root-key、tls-key、signing-key 等秘密，应使用 owner-only 权限，不提交 Git，不放进公开 Release，密钥轮换时同步更新。
