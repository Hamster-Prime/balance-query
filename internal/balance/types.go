// Package balance defines result types, provider interfaces, and plugin config.
package balance

import "time"

// ── Result ───────────────────────────────────────────────────────────────────

// Result is the unified balance/quota snapshot for one provider API key.
type Result struct {
	Provider    string `json:"provider"`
	ProviderKey string `json:"provider_key,omitempty"`
	AuthID      string `json:"auth_id"`
	AccountName string `json:"account_name,omitempty"`
	KeyPreview  string `json:"key_preview,omitempty"`
	BaseURL     string `json:"base_url,omitempty"`

	// HasBalance preserves a legitimate zero balance through JSON omitempty.
	// BalanceScope identifies whether several API keys own independent wallets.
	BalanceUSD   float64 `json:"balance_usd,omitempty"`
	HasBalance   bool    `json:"has_balance,omitempty"`
	BalanceScope string  `json:"balance_scope,omitempty"`

	// BalanceAmount and BalanceCurrency preserve wallets denominated in CNY or
	// provider-defined credits. BalanceUSD remains available to older clients.
	BalanceAmount    float64 `json:"balance_amount,omitempty"`
	BalanceCurrency  string  `json:"balance_currency,omitempty"`
	HasBalanceAmount bool    `json:"has_balance_amount,omitempty"`

	// Historical cost is separate from wallet balance. Claude, for example,
	// reports organization spend rather than remaining credit.
	CostUSD   float64 `json:"cost_usd,omitempty"`
	HasCost   bool    `json:"has_cost,omitempty"`
	CostScope string  `json:"cost_scope,omitempty"`

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

	// Non-empty means this entry failed to fetch. Error is retained for older
	// dashboard clients; Failure carries a stable category and actionable detail.
	Error   string       `json:"error,omitempty"`
	Failure *FailureInfo `json:"failure,omitempty"`
	// Warnings preserve usable partial data when one of several independent
	// provider endpoints fails (for example Anthropic usage vs. cost reports).
	Warnings []FailureInfo `json:"warnings,omitempty"`

	FetchedAt time.Time `json:"fetched_at"`
}

// FailureInfo is the normalized, provider-safe description of a failed query.
// Provider responses are deliberately reduced to these fields so credentials,
// HTML error pages, and other untrusted upstream content are never reflected in
// the dashboard.
type FailureInfo struct {
	Kind         string `json:"kind"`
	Title        string `json:"title"`
	Reason       string `json:"reason"`
	Suggestion   string `json:"suggestion,omitempty"`
	Retryable    bool   `json:"retryable"`
	HTTPStatus   int    `json:"http_status,omitempty"`
	ProviderCode string `json:"provider_code,omitempty"`
	RequestID    string `json:"request_id,omitempty"`
	// RetryAfterSeconds mirrors an upstream Retry-After hint when available.
	RetryAfterSeconds int64 `json:"retry_after_seconds,omitempty"`
}

const (
	FailureAuthentication   = "authentication"
	FailurePermission       = "permission"
	FailureInsufficientFund = "insufficient_funds"
	FailureQuotaExhausted   = "quota_exhausted"
	FailureRateLimited      = "rate_limited"
	FailureConflict         = "conflict"
	FailureInvalidConfig    = "invalid_config"
	FailureEndpoint         = "endpoint"
	FailureProxy            = "proxy"
	FailureTimeout          = "timeout"
	FailureDNS              = "dns"
	FailureTLS              = "tls"
	FailureNetwork          = "network"
	FailureInvalidResponse  = "invalid_response"
	FailureService          = "service_unavailable"
	FailureAccount          = "account_restricted"
	FailureNoData           = "no_data"
	FailureUnsupported      = "unsupported"
	FailureUnknown          = "unknown"
)

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

	// Percent fields are used when an API reports only percentages. UsedPercent
	// is 0-100; boosted plans may legitimately report RemainingPercent above 100.
	UsedPercent      float64 `json:"used_percent,omitempty"`
	RemainingPercent float64 `json:"remaining_percent,omitempty"`
	// CapacityPercent is the effective percentage ceiling for this key. Most
	// plans use 100; boosted plans can expose a larger capacity.
	CapacityPercent float64 `json:"capacity_percent,omitempty"`

	ResetAt        string `json:"reset_at,omitempty"`
	ResetInSeconds int64  `json:"reset_in_seconds,omitempty"`
	Unlimited      bool   `json:"unlimited,omitempty"`
	Unavailable    bool   `json:"unavailable,omitempty"`
	// Unknown means the provider returned the window but omitted enough fields
	// that the allowance cannot be calculated. It is distinct from an explicit
	// zero allowance and from a plan that does not include the resource.
	Unknown bool   `json:"unknown,omitempty"`
	Status  string `json:"status,omitempty"`
	// Blocking marks a quota as a hard gate for sibling windows. When a
	// blocking window is exhausted, capacity remaining in another window does
	// not make the provider usable (for example Kimi's weekly and rolling
	// limits must both have capacity).
	Blocking bool `json:"blocking,omitempty"`
	// ShowUsedWhenUnlimited asks the UI to keep a provider-reported usage
	// counter visible even though the key itself has no enforced cap.
	ShowUsedWhenUnlimited bool `json:"show_used_when_unlimited,omitempty"`

	// AggregationScope is "key" only when each API key owns an independent
	// allowance. Account, organization, and unknown windows must not be summed.
	AggregationScope string `json:"aggregation_scope,omitempty"`
	// AggregationKey gives the dashboard a stable identity for matching the same
	// allowance across keys, independent of API response order.
	AggregationKey string `json:"aggregation_key,omitempty"`
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
