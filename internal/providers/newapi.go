package providers

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Hamster-Prime/balance-query/internal/balance"
)

// NewAPI queries the API-key read-only usage endpoint introduced by New API.
// Older instances fall back to the OpenAI-compatible dashboard billing routes.
type NewAPI struct {
	BaseURL string
}

type newAPITokenUsageResp struct {
	Code    bool   `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Object             string          `json:"object"`
		Name               string          `json:"name"`
		TotalGranted       float64         `json:"total_granted"`
		TotalUsed          float64         `json:"total_used"`
		TotalAvailable     float64         `json:"total_available"`
		UnlimitedQuota     bool            `json:"unlimited_quota"`
		ModelLimits        map[string]bool `json:"model_limits"`
		ModelLimitsEnabled bool            `json:"model_limits_enabled"`
		ExpiresAt          int64           `json:"expires_at"`
	} `json:"data"`
}

type newAPIStatusResp struct {
	Success bool `json:"success"`
	Data    struct {
		QuotaPerUnit               float64 `json:"quota_per_unit"`
		QuotaDisplayType           string  `json:"quota_display_type"`
		USDExchangeRate            float64 `json:"usd_exchange_rate"`
		CustomCurrencySymbol       string  `json:"custom_currency_symbol"`
		CustomCurrencyExchangeRate float64 `json:"custom_currency_exchange_rate"`
	} `json:"data"`
}

type newAPISubscriptionResp struct {
	HardLimit       float64 `json:"hard_limit_usd"`
	SystemHardLimit float64 `json:"system_hard_limit_usd"`
	SoftLimit       float64 `json:"soft_limit_usd"`
	AccessUntil     int64   `json:"access_until"`
	Error           *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type newAPIBillingUsageResp struct {
	TotalUsage float64 `json:"total_usage"`
	Error      *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (n NewAPI) Fetch(authID, token, proxyURL string) balance.Result {
	label := balance.ProviderLabel[balance.ProviderNewAPI]
	if strings.TrimSpace(n.BaseURL) == "" {
		return errResult(authID, label, "所选 AI 提供商没有配置接口地址")
	}
	usageURL, err := serviceEndpoint(n.BaseURL, "/api/usage/token/")
	if err != nil {
		return errResult(authID, label, err.Error())
	}
	var usage newAPITokenUsageResp
	if err := getJSON(usageURL, token, proxyURL, &usage); err == nil {
		if !usage.Code {
			return errResult(authID, label, firstNonEmpty(usage.Message, "New API 返回密钥用量查询错误"))
		}
		var status newAPIStatusResp
		statusURL, _ := serviceEndpoint(n.BaseURL, "/api/status")
		_ = getJSONWithHeaders(statusURL, proxyURL, nil, &status)
		return parseNewAPITokenUsage(authID, usage, status)
	} else if !strings.Contains(strings.ToLower(err.Error()), "http 404") {
		return errResult(authID, label, err.Error())
	}
	return n.fetchLegacyBilling(authID, token, proxyURL)
}

func parseNewAPITokenUsage(authID string, response newAPITokenUsageResp, status newAPIStatusResp) balance.Result {
	label := balance.ProviderLabel[balance.ProviderNewAPI]
	convert, unit := newAPIQuotaConverter(status)
	total := convert(response.Data.TotalGranted)
	used := convert(response.Data.TotalUsed)
	remaining := convert(response.Data.TotalAvailable)
	window := balance.QuotaWindow{
		Group:            "密钥额度",
		Label:            "总额度",
		Used:             used,
		Total:            total,
		Remaining:        remaining,
		Unit:             unit,
		Unlimited:        response.Data.UnlimitedQuota,
		AggregationScope: "key",
	}
	if !window.Unlimited && total > 0 {
		window.UsedPercent = percentFromValues(used, total)
		window.RemainingPercent = clampPercent(100 - window.UsedPercent)
	}
	if response.Data.ExpiresAt > 0 {
		window.ResetAt = formatUnixTimestamp(response.Data.ExpiresAt)
	}

	r := balance.Result{
		Provider:     label,
		AuthID:       authID,
		Plan:         "API 密钥额度",
		ResetAt:      window.ResetAt,
		QuotaWindows: []balance.QuotaWindow{window},
		Extra:        map[string]string{},
		FetchedAt:    time.Now(),
	}
	if response.Data.Name != "" {
		r.Extra["密钥名称"] = response.Data.Name
	}
	if response.Data.UnlimitedQuota {
		r.QuotaDisplay = "密钥额度不设上限"
	} else {
		r.QuotaDisplay = fmt.Sprintf("剩余 %.4f / %.4f %s", remaining, total, unit)
	}
	if response.Data.ExpiresAt > 0 {
		r.Extra["密钥到期"] = window.ResetAt
	}
	if response.Data.ModelLimitsEnabled {
		models := make([]string, 0, len(response.Data.ModelLimits))
		for model, enabled := range response.Data.ModelLimits {
			if enabled {
				models = append(models, model)
			}
		}
		sort.Strings(models)
		if len(models) > 0 {
			r.Extra["允许模型"] = strings.Join(models, "、")
		} else {
			r.Extra["模型限制"] = "已启用，但未返回模型清单"
		}
	} else {
		r.Extra["模型限制"] = "未启用"
	}
	return r
}

func newAPIQuotaConverter(status newAPIStatusResp) (func(float64) float64, string) {
	data := status.Data
	if !status.Success || data.QuotaPerUnit <= 0 {
		return func(value float64) float64 { return value }, "内部额度"
	}
	switch strings.ToUpper(strings.TrimSpace(data.QuotaDisplayType)) {
	case "TOKENS":
		return func(value float64) float64 { return value }, "令牌"
	case "CNY":
		rate := data.USDExchangeRate
		if rate <= 0 {
			rate = 1
		}
		return func(value float64) float64 { return value / data.QuotaPerUnit * rate }, "CNY"
	case "CUSTOM":
		rate := data.CustomCurrencyExchangeRate
		if rate <= 0 {
			rate = 1
		}
		unit := firstNonEmpty(data.CustomCurrencySymbol, "自定义币种")
		return func(value float64) float64 { return value / data.QuotaPerUnit * rate }, unit
	default:
		return func(value float64) float64 { return value / data.QuotaPerUnit }, "USD"
	}
}

func (n NewAPI) fetchLegacyBilling(authID, token, proxyURL string) balance.Result {
	label := balance.ProviderLabel[balance.ProviderNewAPI]
	subscriptionURL, err := serviceEndpoint(n.BaseURL, "/v1/dashboard/billing/subscription")
	if err != nil {
		return errResult(authID, label, err.Error())
	}
	usageURL, err := serviceEndpoint(n.BaseURL, "/v1/dashboard/billing/usage")
	if err != nil {
		return errResult(authID, label, err.Error())
	}
	var subscription newAPISubscriptionResp
	if err := getJSON(subscriptionURL, token, proxyURL, &subscription); err != nil {
		return errResult(authID, label, err.Error())
	}
	if subscription.Error != nil {
		return errResult(authID, label, firstNonEmpty(subscription.Error.Message, "New API 返回订阅查询错误"))
	}
	var usage newAPIBillingUsageResp
	if err := getJSON(usageURL, token, proxyURL, &usage); err != nil {
		return errResult(authID, label, err.Error())
	}
	if usage.Error != nil {
		return errResult(authID, label, firstNonEmpty(usage.Error.Message, "New API 返回用量查询错误"))
	}
	total := subscription.HardLimit
	if total <= 0 {
		total = subscription.SystemHardLimit
	}
	if total <= 0 {
		total = subscription.SoftLimit
	}
	used := usage.TotalUsage / 100
	remaining := maxFloat(total-used, 0)
	window := balance.QuotaWindow{
		Group:            "密钥额度",
		Label:            "总额度（兼容模式）",
		Used:             used,
		Total:            total,
		Remaining:        remaining,
		Unit:             "站点额度",
		UsedPercent:      percentFromValues(used, total),
		RemainingPercent: clampPercent(100 - percentFromValues(used, total)),
		AggregationScope: "account",
	}
	if subscription.AccessUntil > 0 {
		window.ResetAt = formatUnixTimestamp(subscription.AccessUntil)
	}
	r := balance.Result{
		Provider:     label,
		AuthID:       authID,
		QuotaDisplay: fmt.Sprintf("剩余 %.4f / %.4f 站点额度", remaining, total),
		ResetAt:      window.ResetAt,
		QuotaWindows: []balance.QuotaWindow{window},
		Extra: map[string]string{
			"查询模式": "旧版 billing 兼容接口",
			"额度单位": "由 New API 站点设置决定",
		},
		FetchedAt: time.Now(),
	}
	if window.ResetAt != "" {
		r.Extra["密钥到期"] = window.ResetAt
	}
	return r
}
