# MineOps

MineOps 是一款本地优先的 Minecraft 服务器桌面管理工具。你无需长期手动操作 Linux、Java 和 Shell，即可通过图形界面连接远程主机、安装服务端、管理文件、查看控制台并监控运行状态。

## 你可以用 MineOps 做什么

- 保存并管理多个 SSH 连接，检测网络、认证和主机指纹状态。
- 在独立工作区中使用 SSH Terminal 和服务器 Console。
- 浏览、编辑、上传、下载和安全解压远程文件。
- 自动发现远程 Java，或安装受管理的 Java Runtime。
- 创建或导入 Minecraft 服务器，配置版本、内存、目录、参数和 EULA。
- 启动、停止、重启服务器，并识别异常退出和进程身份变化。
- 编辑 `server.properties`，保留原有注释、顺序和未知配置项。
- 通过 SSH 采集主机与 Java 进程指标，无需部署长期运行的监控 Agent。
- 使用 Minecraft spark 查看 TPS、MSPT、健康报告和 Profiler 结果。

## 支持的 Minecraft 服务端

MineOps 当前包含以下服务端目录和安装流程：

- Vanilla
- Paper、Folia
- Purpur
- Spigot
- Fabric、Quilt
- Forge、NeoForge
- Velocity、Waterfall、BungeeCord

不同服务端和 Minecraft 版本对 Java 的要求不同。MineOps 会在安装前计算推荐版本，并检查远程主机中可复用的 Java Runtime。

## 运行环境

### 桌面端

| 平台    | 当前目标                                                          |
| ------- | ----------------------------------------------------------------- |
| Windows | Windows 10 / Server 2016 及以上，首期使用当前用户范围 NSIS 安装器 |
| macOS   | macOS 12.0 及以上，支持 arm64、amd64 或 Universal App             |
| Linux   | AppImage、deb、rpm，要求 GTK4 与 WebKitGTK 6.0                    |

正式安装包仍在发布验收中，具体支持范围以对应版本的 Release Notes 为准。

### 远程服务器

首期只支持远程 Linux 主机。目标主机需要：

- 可用的 OpenSSH Server
- POSIX `sh`
- GNU tar/coreutils
- `curl`
- 可写的用户 Home 目录
- 需要自动调整防火墙时，具备 `root` 或免交互 `sudo -n` 权限

MineOps 会在安装前执行能力探测。探测失败时不会继续创建或覆盖服务器目录。

## 第一次使用

### 1. 添加 SSH Session

在 **SSH Sessions** 中填写主机、端口、用户名和认证方式。首次连接时请仔细核对服务器返回的主机指纹；指纹发生变化时，MineOps 会阻止静默连接。

建议先使用专用 Linux 测试账号，并限制它只能访问测试目录。

### 2. 准备 Java `可选`
打开 **Java Runtimes**，选择 SSH Session 后扫描远程 Java。你可以注册已有 Java 路径，也可以从受信任目录安装托管 JDK。

支持的主要 Java 版本为 8、11、17、21 和 24，实际推荐版本取决于 Minecraft 与服务端类型。

### 3. 创建或导入服务器

在 **MC Servers** 中选择：

- 创建新服务器：选择发行版、版本、Java、内存、远程目录和启动参数。
- 导入现有服务器：检查 Jar、`server.properties`、`eula.txt` 和版本线索，不主动改写原目录。

开始安装前，MineOps 会展示安装摘要和 EULA 提示。确认后，安装会作为后台 Operation 执行；关闭安装窗口不会取消任务。

### 4. 管理服务器

服务器详情页提供以下工作区：

- Overview：状态、版本、Java、内存和最近操作
- Console：实时输出、命令输入、搜索和重新连接
- Files：远程文件管理和文本编辑
- Configuration：结构化或原文编辑 `server.properties`
- Monitoring：CPU、内存、磁盘、网络和 Java 指标
- Performance：TPS、MSPT、Spark 报告与 Profiler
- Install History：安装步骤、日志和失败恢复
- Backups：配置与服务器备份记录

玩家活动和累计在线时长功能仍处于发布前验收阶段，是否进入具体版本以 Release Notes 为准。

## 数据与安全

MineOps 将数据保存在运行应用的本地电脑中：

- 数据库使用 SQLCipher 整库加密。
- 数据库密钥由系统安全存储托管，不要求用户设置数据库主密码。
- SSH 密码、私钥口令和代理凭据不会写入普通配置文件。
- Known Hosts 会记录并校验远程主机指纹。
- 日志、诊断包和前端接口会过滤密码、Token、私钥等敏感内容。
- 备份文件使用 `.mineops-backup` 格式，并包含数据库身份与完整性信息。

即使具备这些保护，也请遵循最小权限原则：使用专用 SSH 账号、定期创建备份，并避免在生产服务器上首次验证新版本。

## 长任务与恢复

安装、下载、上传、启动、停止、重启和备份等操作会显示在 **Operations** 中。任务包含当前阶段、进度、错误和重试信息。

应用异常关闭后，MineOps 会尝试恢复 Operation、安装步骤和远程进程状态。恢复结果仍应由用户核对，尤其是服务器目录、Java 进程和防火墙状态。

## 遇到问题

提交问题时，请提供：

- MineOps 版本
- 桌面操作系统与架构
- 远程 Linux 发行版
- Minecraft 服务端类型与版本
- 可重复的操作步骤
- 已脱敏的错误信息或诊断包

不要公开上传 SSH 密码、私钥、Token、完整数据库或包含敏感信息的日志。

## 许可证

MineOps 自有内容采用 [CC BY-NC-SA 4.0](LICENSE)。
