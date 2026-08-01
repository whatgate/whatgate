# WhatGate 桌面客户端

这是面向普通用户的跨平台桌面客户端，采用 Avalonia 和 XAML 构建，界面风格与 WPF
接近，但可以在 Windows、macOS 和 Linux 上使用。

## 普通用户

1. 下载适合自己系统的安装包并解压。
2. Windows 双击 `Install-WhatGate.cmd`，Linux 运行 `install.sh`，macOS 将
   `WhatGate.app` 拖入“应用程序”。
3. 第一个使用者选择“创建我的网络”；已有网络的成员选择“加入已有网络”。
4. 创建网络不需要邀请码；加入网络时填写管理员提供的协调器地址和邀请码。
5. 保持“仅信任圈”模式，选择地区后启动连接。
6. 将支持 SOCKS5 的应用代理设为 `127.0.0.1:1080`。

邀请码不会写入长期设置。设备身份、已验证目录和普通连接设置保存在当前用户的应用
数据目录中，退出客户端时会停止由它启动的核心进程。

“高级管理”也已完整内嵌到客户端，可直接查看节点详情、控制共享出口、加入信任圈和
认可其他信任圈。桌面客户端不会再打开 `http://127.0.0.1:7070`；该端口仅作为核心与
客户端之间的本机控制接口使用。

创建网络时，客户端会启动随安装包提供的本地协调服务，将当前设备一次性登记为首位
管理员，并自动生成随机成员邀请码。首位成员出现后，无邀请码自举入口立即关闭；后续
成员必须使用已准入成员签名生成的邀请码。局域网协调器使用明文 HTTP，仅应在可信局域网
中使用；跨互联网部署应改用 HTTPS 协调器。

## 开发与构建

```powershell
dotnet build desktop/WhatGate.Desktop/WhatGate.Desktop.csproj

# 构建当前系统和架构的安装包
./scripts/build-desktop.ps1 -Version dev

# 也可以显式指定目标
./scripts/build-desktop.ps1 -Version v1.0.0 -Runtimes win-x64,win-arm64
```

发布包输出到 `dist/desktop/`。Windows 和 Linux 包中带有安装脚本；macOS 输出标准
`.app` 目录。仓库的 Release 工作流会分别在 Windows、Linux、macOS 原生环境构建
x64 与 arm64 包，以保留各系统所需的文件权限。正式公开发布前仍需配置 Windows
代码签名以及 macOS 签名、公证。
