# MineOps 第三方软件声明

MineOps 包含由第三方提供的开源软件。这些组件仍受各自的版权声明和许可证条款约束。
本文件仅用于说明，不替代上游项目随软件发布的正式许可证文本。

## 前端运行时依赖

前端依赖树由 `frontend/package-lock.json` 锁定。当前生产依赖树包含 188 个软件包，
其 SPDX 许可证元数据分布如下：

| 许可证 | 软件包数量 |
| --- | ---: |
| MIT | 157 |
| MPL-2.0 | 12 |
| 0BSD | 5 |
| Apache-2.0 | 4 |
| BSD-3-Clause | 4 |
| ISC | 3 |
| BSD-2-Clause | 2 |
| MPL-2.0 OR Apache-2.0 | 1 |

直接运行时依赖如下：

- `@lucide/vue` 1.24.0
- `@tanstack/vue-query` 5.101.2
- `@wailsio/runtime` 3.0.0-alpha.97
- `@xterm/addon-fit` 0.11.0
- `@xterm/addon-search` 0.16.0
- `@xterm/addon-web-links` 0.12.0
- `@xterm/xterm` 6.0.0
- `echarts` 6.1.0
- `monaco-editor` 0.55.1
- `naive-ui` 2.44.1
- `pinia` 3.0.4
- `vue` 3.5.39
- `vue-echarts` 8.0.1
- `vue-router` 5.1.0

各软件包的版本、解析来源、完整性哈希、传递依赖及许可证标识记录在
`frontend/package-lock.json` 中。对应的许可证文本包含在已安装的软件包及其上游源码
发布内容中。

## Go 依赖

桌面应用及其辅助工具包含 `go.mod` 中列出并由 `go.sum` 锁定的 Go 模块。各模块的
版权及许可证详情由其上游仓库和 Go 工具链下载的模块源码提供。

主要运行时组件包括 Wails、GORM、SQLCipher 绑定、SSH/SFTP 库、密码学库及平台集成
软件包。各组件分别适用其自身的许可证条款。

## 静态资源

随应用打包的字体、图标及其他资源保留其上游项目提供的声明和许可证条款。其中，
Codicons 按 MIT 许可证发布，Lucide 图标按 ISC 许可证发布。

## 源码与许可证查询

如需确认某次构建使用的确切依赖集合，请查阅对应 MineOps 源码版本附带的锁文件。
上游许可证文本也可通过各软件包的解析来源地址或模块仓库获取。
