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

	// QuotaWindows contains every independent allowance window returned by the
	// provider (for example 5 hours, 7 days, weekly, or monthly).  Float values
	// are intentional: some compatible services account in money/credits rather
	// than integer tokens.
	QuotaWindows []QuotaWindow `json:"quota_windows,omitempty"`

	// Non-empty means this entry failed to fetch.
	Error string `json:"error,omitempty"`

	FetchedAt time.Time `json:"fetched_at"`
}

// QuotaWindow is one independently-resetting quota bucket. Providers often
// expose several of these at once, so flattening them into Result's legacy
// token fields loses important information.
type QuotaWindow struct {
	// Group separates windows belonging to different models or resources.
	Group string `json:"group,omitempty"`
	Label string `json:"label"`

	Used      float64 `json:"used,omitempty"`
	Total     float64 `json:"total,omitempty"`
	Remaining float64 `json:"remaining,omitempty"`
	Unit      string  `json:"unit,omitempty"`

	// Percent fields are used when an API reports only percentages. Values are
	// in the 0-100 range.
	UsedPercent      float64 `json:"used_percent,omitempty"`
	RemainingPercent float64 `json:"remaining_percent,omitempty"`

	ResetAt        string `json:"reset_at,omitempty"`
	ResetInSeconds int64  `json:"reset_in_seconds,omitempty"`
	Unlimited      bool   `json:"unlimited,omitempty"`
	Unavailable    bool   `json:"unavailable,omitempty"`
	Status         string `json:"status,omitempty"`
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
	ProviderClaudeAdmin         ProviderType = "claude_admin"
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
	ProviderClaudeAdmin:         "Claude 用量与成本（管理员密钥）",
	ProviderDeepSeek:            "DeepSeek 官方 API",
	ProviderGLMZAI:              "GLM Coding Plan（Z.AI）",
	ProviderGLMZhipu:            "GLM Coding Plan（智谱 BigModel）",
	ProviderNewAPI:              "New API",
	ProviderKimiAPI:             "Kimi 官方 API",
	ProviderKimiCode:            "Kimi Coding Plan",
	ProviderLongcat:             "LongCat",
	ProviderMiniMaxAPI:          "MiniMax 官方 API（按量）",
	ProviderMiniMaxCodingCN:     "MiniMax Token Plan（国内）",
	ProviderMiniMaxCodingGlobal: "MiniMax Token Plan（海外）",
	ProviderOpenCode:            "OpenCode",
	ProviderVolcengine:          "火山引擎 Coding Plan",
	ProviderXiaomiAPI:           "小米 MiMo API",
	ProviderXiaomiToken:         "小米 Token Plan",
}

// AllProviders returns all ProviderType values in display order.
func AllProviders() []ProviderType {
	return []ProviderType{
		ProviderSub2API,
		ProviderClaudeAdmin,
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
