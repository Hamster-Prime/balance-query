# balance-query — CLIProxyAPI Plugin

[![Release](https://img.shields.io/github/v/release/Hamster-Prime/balance-query)](https://github.com/Hamster-Prime/balance-query/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

一个 [CLIProxyAPI (CPA)](https://github.com/router-for-me/CLIProxyAPI) 插件，查询多个 AI 平台账户的余额 / 额度，并在统一的 Web 仪表盘或 CLI 表格中展示。

## 通过 CPA 插件商店安装（推荐）

在 CPA 的 `config.yaml` 中添加以下内容：

```yaml
plugins:
  enabled: true
  store-sources:
    - "https://raw.githubusercontent.com/Hamster-Prime/balance-query/main/registry-v2.json"
```

重启 CPA 后，在 Web 管理界面的插件商店中找到 **Balance Query** 并点击安装即可。

## 手动安装

1. 前往 [Releases](https://github.com/Hamster-Prime/balance-query/releases) 下载对应平台的 `.zip`
2. 解压得到共享库文件（`.so` / `.dll` / `.dylib`）
3. 将文件放入 CPA 的 `plugins/` 目录
4. 在 `config.yaml` 中启用：

```yaml
plugins:
  enabled: true
  configs:
    balance-query:
      enabled: true
```

## 使用方法

1. 重启 CPA 后，进入 Web 管理界面 → **Balance Query**
2. 点击右上角 **⚙ 设置**
3. 为每个 CPA 账户从下拉菜单中选择对应的 Provider 类型
4. 对于 Sub2API 和 New API，还需填写实例的 Base URL（因为你可能有多个实例）
5. 点击 **保存配置**，仪表盘将自动刷新并显示余额数据

CLI 用法：

```bash
cpa balance            # 打印余额表格
cpa balance-refresh    # 强制刷新缓存后打印
```

## 支持的 Provider

| Provider 类型 | 说明 |
|---|---|
| `sub2api` | Sub2API 实例（需填 Base URL） |
| `newapi` | New API / One API 实例（需填 Base URL） |
| `deepseek` | DeepSeek 官方 API |
| `glm_zai` | GLM Coding Plan — Z.AI 平台 |
| `glm_zhipu` | GLM Coding Plan — 智谱 AI 平台 |
| `kimi_api` | Kimi 官方 API（Moonshot） |
| `kimi_code` | Kimi Coding Plan |
| `longcat` | Longcat Chat |
| `minimax_api` | MiniMax 官方 API |
| `minimax_coding_cn` | MiniMax Coding Plan（国内） |
| `minimax_coding_global` | MiniMax Coding Plan（海外） |
| `opencode` | OpenCode AI |
| `volcengine` | 火山引擎 Coding Plan |
| `xiaomi_api` | 小米 MiMo API |
| `xiaomi_token` | 小米 MiMo Token Plan |

## 项目结构

```
main.go                  # CGo ABI 入口、方法分发、fetch 逻辑
internal/
  balance/types.go       # ProviderType 常量、AuthMapping、PluginConfig
  cache/cache.go         # 泛型 TTL 缓存 Cache[K,V]
  providers/
    registry.go          # providers.Build(ProviderType, baseURL) 工厂
    http.go              # 共用 HTTP 工具
    deepseek.go / glm.go / kimi.go / sub2api.go / newapi.go
    minimax.go / longcat.go / opencode.go / volcengine.go / xiaomi.go
  ui/dashboard.go        # HTML 仪表盘 + 设置页面 + CLI 表格渲染
.github/workflows/
  release.yml            # 打 tag 后自动交叉编译 6 个平台并发布 Release
registry.json            # CPA 插件商店注册表 v1（CI 自动更新）
registry-v2.json         # CPA 插件商店注册表 v2，含 sha256（CI 自动更新）
```

## 构建

需要 Go 1.22 + CGo。

```bash
make
# 或手动：
CGO_ENABLED=1 go build -buildmode=c-shared -trimpath -ldflags="-s -w" -o bin/balance-query.so .
```

## 配置持久化

Provider 映射存储在 `balance-query-config.json` 中，通过 CPA 的 `host.auth.save` 接口持久化，无需手动编辑文件。

缓存 TTL（默认 300 秒）可在仪表盘顶部修改，立即生效，无需重启。

## License

MIT
