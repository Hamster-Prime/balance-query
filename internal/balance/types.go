// Package balance defines result types, provider interface, and per-auth config.
package balance

import "time"

// ── Result ───────────────────────────────────────────────────────────────────

// Result is the unified balance/quota snapshot for one auth entry.
type Result struct {
	Provider string `json:"provider"`
	AuthID   string `json:"auth_id"`

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
	Fetch(authID, token string) Result
}

// ── ProviderType ─────────────────────────────────────────────────────────────

// ProviderType is the canonical identifier for a supported provider.
type ProviderType string

const (
	ProviderSub2API              ProviderType = "sub2api"
	ProviderDeepSeek             ProviderType = "deepseek"
	ProviderGLMZAI               ProviderType = "glm_zai"
	ProviderGLMZhipu             ProviderType = "glm_zhipu"
	ProviderNewAPI               ProviderType = "newapi"
	ProviderKimiAPI              ProviderType = "kimi_api"
	ProviderKimiCode             ProviderType = "kimi_code"
	ProviderLongcat              ProviderType = "longcat"
	ProviderMiniMaxAPI           ProviderType = "minimax_api"
	ProviderMiniMaxCodingCN      ProviderType = "minimax_coding_cn"
	ProviderMiniMaxCodingGlobal  ProviderType = "minimax_coding_global"
	ProviderOpenCode             ProviderType = "opencode"
	ProviderVolcengine           ProviderType = "volcengine"
	ProviderXiaomiAPI            ProviderType = "xiaomi_api"
	ProviderXiaomiToken          ProviderType = "xiaomi_token"
)

// ProviderLabel maps each ProviderType to a human-readable display name.
var ProviderLabel = map[ProviderType]string{
	ProviderSub2API:             "Sub2API",
	ProviderDeepSeek:            "DeepSeek 官方 API",
	ProviderGLMZAI:              "GLM Coding Plan (Z.AI)",
	ProviderGLMZhipu:            "GLM Coding Plan (Zhipu/bigmodel)",
	ProviderNewAPI:              "New API",
	ProviderKimiAPI:             "Kimi 官方 API",
	ProviderKimiCode:            "Kimi Coding Plan",
	ProviderLongcat:             "Longcat",
	ProviderMiniMaxAPI:          "MiniMax 官方 API (国内)",
	ProviderMiniMaxCodingCN:     "MiniMax Coding Plan (国内)",
	ProviderMiniMaxCodingGlobal: "MiniMax Coding Plan (海外)",
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

// NeedsBaseURL returns true for providers that require a custom base URL.
func NeedsBaseURL(p ProviderType) bool {
	return p == ProviderSub2API || p == ProviderNewAPI
}

// ── PluginConfig ─────────────────────────────────────────────────────────────

// AuthMapping holds the user's explicit choice for one CPA auth entry.
type AuthMapping struct {
	Provider ProviderType `json:"provider"`
	// BaseURL is required for sub2api and newapi; ignored for other providers.
	BaseURL string `json:"base_url,omitempty"`
}

// PluginConfig is the full persisted configuration for this plugin,
// stored as a CPA auth file named "balance-query-config.json".
type PluginConfig struct {
	// Mappings maps auth_index → provider assignment.
	Mappings map[string]AuthMapping `json:"mappings"`
}

// ConfigFileName is the name used when saving config via host.auth.save.
const ConfigFileName = "balance-query-config.json"
