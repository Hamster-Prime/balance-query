# 余额与配额 - CLIProxyAPI 插件

[![Release](https://img.shields.io/github/v/release/Hamster-Prime/balance-query)](https://github.com/Hamster-Prime/balance-query/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

用于 [CLIProxyAPI（CPA）](https://github.com/router-for-me/CLIProxyAPI) 的余额、用量与套餐配额查询插件。插件直接读取 CPA 面板“AI 提供商”中的 OpenAI 兼容、Claude、xAI、Codex 和 Gemini 配置，在统一的中文页面中展示账户余额、配额周期、历史用量和重置时间。

## 安装

在 CPA 的 `config.yaml` 中加入插件源：

```yaml
plugins:
  enabled: true
  store-sources:
    - "https://raw.githubusercontent.com/Hamster-Prime/balance-query/main/registry-v2.json"
```

重启 CPA 后，在插件商店中安装“余额与配额”。

也可以从 [Releases](https://github.com/Hamster-Prime/balance-query/releases) 下载对应平台压缩包，解压后将 `.so`、`.dylib` 或 `.dll` 放入 CPA 的 `plugins/` 目录。

## 使用

1. 先在 CPA 的“AI 提供商”中配置服务地址和接口密钥。
2. 打开插件菜单“余额与配额”，切换到“查询设置”。
3. 在 OpenAI 兼容、Claude、xAI、Codex 或 Gemini 分组中，为每个服务地址手动选择查询类型并保存。
4. 返回“概览”查看结果，或使用刷新按钮绕过缓存重新查询。

Sub2API、New API 等自建实例直接使用所选 AI 提供商中的服务地址和密钥，无需在插件配置中重复填写。相同大类下按服务地址分组；同一地址配置多条 API Key 时，插件会逐条查询并分别展示。查询优先使用每条密钥自己的代理地址，未单独设置时继承 CPA 全局代理，并支持 `direct` / `none` 显式直连。已停用的 OpenAI 兼容提供商仍会出现在查询设置中，但不会发起查询。

页面会同步 CPA 的暖灰、纯白和深色主题以及 CSS 主题变量，并监听 CPA 运行时主题切换。进入、卡片、进度条和加载动画遵循 CPA 的动画节奏与系统“减少动态效果”设置。同源面板会自动复用当前管理会话；跨域面板或未保存管理密码时，页面会显示临时连接表单，密钥仅保留在当前页面内存中。

配额结果按模型或资源分组，完整展示接口实际返回的短周期、日、周、月窗口，包括已用、剩余、总量、百分比、重置时间和倒计时；不限量及“不在当前套餐”也会单独标记。账户卡片一行一个，余额和核心配额默认平铺展示；完整账户明细不截断，但默认收起，可从卡片右侧平滑展开。

## 查询类型

| 查询类型 | 状态 | 展示内容 |
|---|---|---|
| Sub2API | 可查询 | 总额度、5 小时/每日/7 天限额、日/周/月订阅额度、今日/累计用量、近 30 天与模型明细 |
| Claude 用量与成本（管理员密钥） | 可查询 | 近 30 天组织费用、输入/缓存/输出令牌、Web 搜索、模型和费用分类；需要 Claude 管理员 API 密钥 |
| New API | 可查询 | 密钥总量/已用/剩余、不限量、模型白名单、到期时间；按站点 USD/CNY/令牌/自定义单位换算，兼容旧版 billing 接口 |
| DeepSeek 官方 API | 可查询 | 各币种总余额、赠送余额、充值余额与账户状态 |
| GLM Coding Plan（Z.AI） | 可查询 | 5 小时、周、MCP 月额度、重置时间，以及近 24 小时模型/令牌/MCP 明细 |
| GLM Coding Plan（智谱） | 可查询 | 同上，使用 BigModel 官方区域端点 |
| Kimi 官方 API | 可查询 | 可用、现金与赠金余额；域名跟随所选 AI 提供商 |
| Kimi Coding Plan | 可查询 | 官方 CLI 同口径的周配额与滚动 5 小时配额百分比、重置时间、加量包余额及月付费上限 |
| MiniMax Token Plan（国内） | 可查询 | 官方 CLI 同款的各模型当前周期、周额度、剩余比例、倍率与重置倒计时 |
| MiniMax Token Plan（海外） | 可查询 | 同上，使用海外区域端点 |

查询设置只列出有公开、可验证接口的查询类型。MiniMax 按量 API、小米 API/Token Plan、LongCat、OpenCode 和火山引擎等仅能通过登录控制台查看的类型不会出现在下拉选项中。xAI、Codex 和 Gemini 会作为 CPA 服务来源显示；其官方标准模型密钥没有余额接口时，可按实际服务地址手动选择兼容的 Sub2API、New API 等查询类型。

## 数据与安全

- 提供商列表来自 CPA 受认证的 OpenAI 兼容、Claude、xAI、Codex 和 Gemini 配置接口，不读取认证文件。
- 手动映射保存在 `plugins.configs.balance-query.provider_mappings`，不创建额外 JSON 认证文件。
- API Key 只在浏览器到插件受认证查询路由的单次请求中传递，不写入页面 HTML、插件配置或日志。
- 内存缓存键使用 API Key 的 SHA-256 摘要，不包含明文密钥；失败结果不会缓存。
- 为了在 CPA 面板内免重复登录，同源页面会读取 CPAMC 当前保存的管理会话，并仅在页面内存中使用管理密钥；这是当前 CPA 尚未提供正式插件认证桥时的兼容方案。安装插件等同于信任该插件拥有当前实例的管理 API 权限，请只从可信发布源安装。
- 页面不加载第三方脚本、字体或远程图片；动态数据使用文本节点渲染，上游英文错误会在服务端转换为中文分类提示后再展示。
- 对官方未公开 API-Key 账单端点的平台，查询设置不提供对应选项，也不会向推测的控制台路径发送密钥。
- 默认缓存 300 秒，可在“查询设置”中配置为 `0`（关闭）或 `10-86400` 秒。

## 构建与测试

需要 Go 1.22 和 CGo：

```bash
go test ./...
make build
```

构建产物位于 `bin/balance-query.so`。推送 `v*` 标签后，GitHub Actions 会测试并构建 Linux、macOS、Windows 的 amd64/arm64 产物，同时更新插件商店注册表。

## License

[MIT](LICENSE)
