# balance-query — CLIProxyAPI Plugin

A [CLIProxyAPI (CPA)](https://github.com/router-for-me/CLIProxyAPI) plugin written in Go that queries the balance / quota of multiple AI platform accounts and displays them in a unified Web dashboard or CLI table.

## Supported Providers

| Provider | 说明 |
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

## Architecture

```
main.go                  # CGo ABI entry point, method dispatch, fetch logic
internal/
  balance/types.go       # ProviderType constants, AuthMapping, PluginConfig
  cache/cache.go         # Generic TTL cache (Cache[K,V])
  providers/
    registry.go          # providers.Build(ProviderType, baseURL) factory
    http.go              # shared HTTP helpers
    deepseek.go
    glm.go
    kimi.go
    sub2api.go
    newapi.go
    minimax.go
    longcat.go
    opencode.go
    volcengine.go
    xiaomi.go
  ui/dashboard.go        # HTML dashboard + settings page + CLI table renderer
```

## Building

Requires Go 1.22 and CGo.

```bash
make
# or manually:
CGO_ENABLED=1 go build -buildmode=c-shared -trimpath -ldflags="-s -w" -o bin/balance-query.so .
```

The output is `bin/balance-query.so` (a C-ABI shared library loaded by CPA).

## Usage

1. Copy `bin/balance-query.so` into your CPA plugins directory.
2. Restart CPA.
3. In the CPA Web UI, navigate to **Balance Query → Settings**.
4. For each CPA account, select the corresponding provider type from the dropdown. For `sub2api` and `newapi`, also enter the instance Base URL.
5. Click **保存配置** — the dashboard will refresh and show live balance data.

CLI:

```bash
cpa balance          # print balance table
cpa balance-refresh  # force-refresh cache first
```

## Configuration

Provider mappings are stored in `balance-query-config.json` via `host.auth.save` (CPA's built-in config persistence). No manual file editing is needed.

The cache TTL (default 300 s) can be adjusted in the dashboard header and is applied immediately without restart.

## License

MIT
