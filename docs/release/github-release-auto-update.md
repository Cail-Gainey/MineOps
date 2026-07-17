# GitHub Release 自动发布与桌面更新

MineOps 使用受保护的 `release` GitHub Environment 从 `v<semver>` 标签发布桌面 Release。正式版本使用 `v1.2.3`，Beta 使用 `v1.2.3-beta.1`；带预发布段的标签会发布为 GitHub prerelease，并写入 `beta` 更新通道。

## 受保护环境与密钥

`release` Environment 应限制到受保护标签，并配置审批人。自动更新信任链只要求以下两个 Secrets：

- `UPDATE_PRIVATE_KEY`、`UPDATE_PUBLIC_KEY`：由固定版本 `wails3 updater genkey` 生成；私钥仅交给 publish job，公钥必须与客户端内嵌值一致。

更新签名不依赖商业 CA。任何维护者都可以免费生成自己的 Ed25519 密钥：

```sh
task update:generate-key
```

默认密钥位于被 Git 忽略的 `.keys/` 目录。将 `.keys/mineops-update.key.pub` 的内容配置为
`UPDATE_PUBLIC_KEY`，将私钥配置为受保护环境中的
`UPDATE_PRIVATE_KEY`。私钥只用于 CI 生成 Artifact/Manifest 签名，客户端只内嵌公钥。

Windows Authenticode 和 Apple Developer ID/公证凭据均为可选项。未配置时，CI 仍会发布可用的
NSIS/DMG/ZIP/便携二进制；macOS 使用免费的 ad-hoc 签名，更新安全性由上述 Ed25519 Manifest
和 Artifact 签名保证。Linux 包也不要求额外 PGP 证书。

更新私钥需要离线加密备份，不能提交到仓库。丢失私钥后，已安装客户端无法信任用新密钥签发的更新；密钥轮换必须先发布支持多公钥的过渡版本。

## Release 资产

资产名固定为 `MineOps-<version>-<platform>-<arch>[.<ext>]`：

- Windows：由 Ed25519 更新签名保护的原始 EXE（自更新）和 `-installer.exe`（首次安装）；Authenticode 可选。
- macOS：每个架构同时发布 `.dmg` 和 ZIP；归档内只有一个顶层 `MineOps.app`。商业签名/公证凭据可选。
- Linux：可写的便携二进制（自更新），以及 AppImage、deb、rpm（安装或手工更新）。
- `mineops-desktop-update.json`、`mineops-desktop-update.sig.json`、`SHA512SUMS`、`LICENSE`、`THIRD_PARTY_NOTICES.md`。

deb/rpm 属于包管理器管理的安装，不允许客户端直接替换。AppImage 默认也不启用自动替换；只有在真实 AppImage 内运行 `build/release/appimage-target-spike.sh`，确认 `$APPIMAGE` 指向可写的外层文件而 `/proc/self/exe` 指向挂载运行时后，才可重新评估。当前回退路径是打开 GitHub Release 手动下载。

## 发布和重跑

推送标签会自动执行原生平台构建、签名、打包、更新 Manifest 双重签名验证和包冒烟检查。publish job 是唯一拥有 `contents: write` 的任务。流水线先创建或刷新 draft，所有门禁通过后才公开。

同一标签重跑可以确定性刷新 draft 的完整资产集合。已经公开的 Release 默认不可变；只有手工触发工作流并明确启用 `allow_public_maintenance` 才能维护公开资产，且应在审计记录中说明原因。

回滚不通过修改已公开资产或降级 Manifest 完成。发现问题时应撤下有问题的 Release、发布更高补丁版本，并在 Release Notes 中说明人工恢复方式。
