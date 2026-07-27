# 余额与配额 - CLIProxyAPI 插件

[![Release](https://img.shields.io/github/v/release/Hamster-Prime/balance-query)](https://github.com/Hamster-Prime/balance-query/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

用于 [CLIProxyAPI（CPA）](https://github.com/router-for-me/CLIProxyAPI) 的余额与套餐配额查询插件。插件以 CPA 面板中“AI 提供商 -> OpenAI 兼容提供商”的条目为数据源，在统一的中文页面中展示余额、用量窗口和重置时间。

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

1. 先在 CPA 的“AI 提供商”中配置一个或多个 OpenAI 兼容提供商。
2. 打开插件菜单“余额与配额”，切换到“查询设置”。
3. 为每个 OpenAI 兼容提供商手动选择余额查询类型并保存。
4. 返回“概览”查看结果，或使用刷新按钮绕过缓存重新查询。

Sub2API、New API 等自建实例直接使用各自 OpenAI 兼容提供商中的 `base-url` 和 `api-key-entries`，无需在插件配置中重复填写地址。一个提供商配置多条 API Key 时，插件会逐条查询并分别展示；查询会优先使用每条密钥自己的 `proxy-url`，未单独设置时继承 CPA 全局代理，并支持 `direct` / `none` 显式直连。已停用的提供商仍会出现在查询设置中，但不会发起查询。

页面会同步 CPA 的暖灰、纯白和深色主题，并遵循 CPA 的过渡时长与“减少动态效果”系统设置。同源面板会自动复用当前管理会话；跨域面板或未保存管理密码时，页面会显示临时连接表单，密钥仅保留在当前页面内存中。

## 查询类型

| 查询类型 | 用途 |
|---|---|
| Sub2API | Sub2API 实例 |
| New API | New API / One API 兼容实例 |
| DeepSeek 官方 API | DeepSeek 余额 |
| GLM Coding Plan（Z.AI） | Z.AI 套餐配额 |
| GLM Coding Plan（智谱） | BigModel 套餐配额 |
| Kimi 官方 API | Moonshot 按量余额 |
| Kimi Coding Plan | Kimi Code 套餐配额 |
| LongCat | LongCat 余额或套餐 |
| MiniMax 官方 API | MiniMax 国内按量余额 |
| MiniMax Coding Plan（国内） | MiniMax 国内套餐 |
| MiniMax Coding Plan（海外） | MiniMax 海外套餐 |
| OpenCode | OpenCode 余额 |
| 火山引擎 Coding Plan | 火山方舟套餐 |
| 小米 MiMo API | 小米按量余额 |
| 小米 Token Plan | 小米 Token 套餐 |

## 数据与安全

- 提供商列表来自 CPA 受认证接口 `GET /v0/management/openai-compatibility`，不读取认证文件。
- 手动映射保存在 `plugins.configs.balance-query.provider_mappings`，不创建额外 JSON 认证文件。
- API Key 只在浏览器到插件受认证查询路由的单次请求中传递，不写入页面 HTML、插件配置或日志。
- 内存缓存键使用 API Key 的 SHA-256 摘要，不包含明文密钥；失败结果不会缓存。
- 为了在 CPA 面板内免重复登录，同源页面会读取 CPAMC 当前保存的管理会话，并仅在页面内存中使用管理密钥；这是当前 CPA 尚未提供正式插件认证桥时的兼容方案。安装插件等同于信任该插件拥有当前实例的管理 API 权限，请只从可信发布源安装。
- 页面不加载第三方脚本、字体或远程图片；动态数据使用文本节点渲染，上游英文错误会在服务端转换为中文分类提示后再展示。
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
