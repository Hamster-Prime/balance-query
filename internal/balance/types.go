// Package balance defines result types, provider interfaces, and plugin config.
package balance

import "time"

// ── Result ───────────────────────────────────────────────────────────────────

// Result is the unified balance/quota snapshot for one provider API key.
type Result struct {
	Provider    string `json:"provider"`
	AuthID      string `json:"auth_id"`
	AccountName string `json:"account_name,omitempty"`
	KeyPreview  string `json:"key_preview,omitempty"`
	BaseURL     string `json:"base_url,omitempty"`

	// Monetary balance; -1 if not applicable.
	BalanceUSD float64 `json:"balance_usd,omitempty"`

	// Token quota (Coding Plan style).
	TokensTotal     int64 `json:"tokens_total,omitempty"`
	TokensUsed      int64 `json:"tokens_used,omitempty"`
	TokensRemaining int64 `json:"tokens_remaining,omitempty"`

	// Human-readable summary line.
	QuotaDisplay string `json:"quota_display,omitempty"`

	// Subscription tier label.
	Plan string `json:"plan,omitempty"`

	// When this window resets (RFC3339 or human-readable string).
	ResetAt string `json:"reset_at,omitempty"`

	// Additional provider-specific key-value pairs rendered in the card.
	Extra map[string]string `json:"extra,omitempty"`

	// Non-empty means this entry failed to fetch.
	Error string `json:"error,omitempty"`

	FetchedAt time.Time `json:"fetched_at"`
}

// ── Fetcher ──────────────────────────────────────────────────────────────────

// Fetcher is implemented by each provider package.
type Fetcher interface {
	Fetch(authID, token, proxyURL string) Result
}

// ── ProviderType ─────────────────────────────────────────────────────────────

// ProviderType is the canonical identifier for a supported provider.
type ProviderType string

const (
	ProviderSub2API             ProviderType = "sub2api"
	ProviderDeepSeek            ProviderType = "deepseek"
	ProviderGLMZAI              ProviderType = "glm_zai"
	ProviderGLMZhipu            ProviderType = "glm_zhipu"
	ProviderNewAPI              ProviderType = "newapi"
	ProviderKimiAPI             ProviderType = "kimi_api"
	ProviderKimiCode            ProviderType = "kimi_code"
	ProviderLongcat             ProviderType = "longcat"
	ProviderMiniMaxAPI          ProviderType = "minimax_api"
	ProviderMiniMaxCodingCN     ProviderType = "minimax_coding_cn"
	ProviderMiniMaxCodingGlobal ProviderType = "minimax_coding_global"
	ProviderOpenCode            ProviderType = "opencode"
	ProviderVolcengine          ProviderType = "volcengine"
	ProviderXiaomiAPI           ProviderType = "xiaomi_api"
	ProviderXiaomiToken         ProviderType = "xiaomi_token"
)

// ProviderLabel maps each ProviderType to a human-readable display name.
var ProviderLabel = map[ProviderType]string{
	ProviderSub2API:             "Sub2API",
	ProviderDeepSeek:            "DeepSeek 官方 API",
	ProviderGLMZAI:              "GLM Coding Plan（Z.AI）",
	ProviderGLMZhipu:            "GLM Coding Plan（智谱 BigModel）",
	ProviderNewAPI:              "New API",
	ProviderKimiAPI:             "Kimi 官方 API",
	ProviderKimiCode:            "Kimi Coding Plan",
	ProviderLongcat:             "Longcat",
	ProviderMiniMaxAPI:          "MiniMax 官方 API（国内）",
	ProviderMiniMaxCodingCN:     "MiniMax Coding Plan（国内）",
	ProviderMiniMaxCodingGlobal: "MiniMax Coding Plan（海外）",
	ProviderOpenCode:            "Open Code",
	ProviderVolcengine:          "火山引擎 Coding Plan",
	ProviderXiaomiAPI:           "小米 MiMo API",
	ProviderXiaomiToken:         "小米 Token Plan",
}

// AllProviders returns all ProviderType values in display order.
func AllProviders() []ProviderType {
	return []ProviderType{
		ProviderSub2API,
		ProviderDeepSeek,
		ProviderGLMZAI,
		ProviderGLMZhipu,
		ProviderNewAPI,
		ProviderKimiAPI,
		ProviderKimiCode,
		ProviderLongcat,
		ProviderMiniMaxAPI,
		ProviderMiniMaxCodingCN,
		ProviderMiniMaxCodingGlobal,
		ProviderOpenCode,
		ProviderVolcengine,
		ProviderXiaomiAPI,
		ProviderXiaomiToken,
	}
}

// PluginConfig is persisted under plugins.configs.balance-query by CPA.
type PluginConfig struct {
	CacheTTLSeconds  int                     `json:"cache_ttl_seconds" yaml:"cache_ttl_seconds"`
	ProviderMappings map[string]ProviderType `json:"provider_mappings" yaml:"provider_mappings"`
}

// IsKnownProvider reports whether p is a supported balance query type.
func IsKnownProvider(p ProviderType) bool {
	for _, candidate := range AllProviders() {
		if p == candidate {
			return true
		}
	}
	return false
}
