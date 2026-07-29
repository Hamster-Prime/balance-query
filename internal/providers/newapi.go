package providers

import (
	"encoding/json"
	"errors"
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
	Code           bool                 `json:"code"`
	Message        string               `json:"message"`
	Error          *newAPIErrorEnvelope `json:"error"`
	HasQuotaFields bool                 `json:"-"`
	HasUsedField   bool                 `json:"-"`
	Data           struct {
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

func (response *newAPITokenUsageResp) UnmarshalJSON(data []byte) error {
	type wireResponse newAPITokenUsageResp
	var decoded wireResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*response = newAPITokenUsageResp(decoded)
	var envelope struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return err
	}
	completeFiniteQuota := true
	for _, key := range []string{"total_granted", "total_used", "total_available"} {
		raw, exists := envelope.Data[key]
		valid := exists && validJSONNumber(raw)
		if key == "total_used" && valid {
			response.HasUsedField = true
		}
		if !valid {
			completeFiniteQuota = false
		}
	}
	explicitUnlimited := false
	if raw, exists := envelope.Data["unlimited_quota"]; exists {
		_ = json.Unmarshal(raw, &explicitUnlimited)
	}
	response.HasQuotaFields = completeFiniteQuota || explicitUnlimited
	return nil
}

func validJSONNumber(raw json.RawMessage) bool {
	if len(raw) == 0 || string(raw) == "null" {
		return false
	}
	var value float64
	return json.Unmarshal(raw, &value) == nil
}

type newAPIStatusResp struct {
	Success bool `json:"success"`
	Data    struct {
		QuotaPerUnit               float64 `json:"quota_per_unit"`
		QuotaDisplayType           string  `json:"quota_display_type"`
		USDExchangeRate            float64 `json:"usd_exchange_rate"`
		CustomCurrencySymbol       string  `json:"custom_currency_symbol"`
		CustomCurrencyExchangeRate float64 `json:"custom_currency_exchange_rate"`
		DisplayInCurrency          *bool   `json:"display_in_currency"`
	} `json:"data"`
}

type newAPISubscriptionResp struct {
	HardLimit       float64              `json:"hard_limit_usd"`
	SystemHardLimit float64              `json:"system_hard_limit_usd"`
	SoftLimit       float64              `json:"soft_limit_usd"`
	AccessUntil     int64                `json:"access_until"`
	HasLimitField   bool                 `json:"-"`
	Error           *newAPIErrorEnvelope `json:"error"`
}

type newAPIBillingUsageResp struct {
	TotalUsage    float64              `json:"total_usage"`
	HasTotalUsage bool                 `json:"-"`
	Error         *newAPIErrorEnvelope `json:"error"`
}

type newAPIErrorEnvelope struct {
	Message string
	Code    any
	Type    string
}

func (upstreamError *newAPIErrorEnvelope) UnmarshalJSON(data []byte) error {
	var message string
	if err := json.Unmarshal(data, &message); err == nil {
		upstreamError.Message = message
		return nil
	}
	var object struct {
		Message string `json:"message"`
		Detail  string `json:"detail"`
		Reason  string `json:"reason"`
		Code    any    `json:"code"`
		Type    string `json:"type"`
	}
	if err := json.Unmarshal(data, &object); err != nil {
		return err
	}
	upstreamError.Message = firstNonEmpty(object.Message, object.Detail, object.Reason)
	upstreamError.Code = object.Code
	upstreamError.Type = object.Type
	return nil
}

func (upstreamError *newAPIErrorEnvelope) providerError(fallback string, secrets ...string) error {
	if upstreamError == nil {
		return nil
	}
	code, _, _ := businessCodeValue(upstreamError.Code)
	code = firstNonEmpty(code, upstreamError.Type)
	return providerBusinessError(code, firstNonEmpty(upstreamError.Message, fallback), secrets...)
}

func (response *newAPISubscriptionResp) UnmarshalJSON(data []byte) error {
	type wireResponse newAPISubscriptionResp
	var decoded wireResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*response = newAPISubscriptionResp(decoded)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, key := range []string{"hard_limit_usd", "system_hard_limit_usd", "soft_limit_usd"} {
		if raw, exists := fields[key]; exists && validJSONNumber(raw) {
			response.HasLimitField = true
			break
		}
	}
	return nil
}

func (response *newAPIBillingUsageResp) UnmarshalJSON(data []byte) error {
	type wireResponse newAPIBillingUsageResp
	var decoded wireResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*response = newAPIBillingUsageResp(decoded)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if raw, exists := fields["total_usage"]; exists && validJSONNumber(raw) {
		response.HasTotalUsage = true
	}
	return nil
}

func (n NewAPI) Fetch(authID, token, proxyURL string) balance.Result {
	label := balance.ProviderLabel[balance.ProviderNewAPI]
	if strings.TrimSpace(n.BaseURL) == "" {
		return errResult(authID, label,
			newProviderError(balance.FailureInvalidConfig, "所选 AI 提供商没有配置接口地址", 0, "", nil))
	}
	usageURL, err := serviceEndpoint(n.BaseURL, "/api/usage/token/")
	if err != nil {
		return errResult(authID, label, err)
	}
	var usage newAPITokenUsageResp
	if err := getJSON(usageURL, token, proxyURL, &usage); err == nil {
		if usage.Error != nil {
			return errResult(authID, label, usage.Error.providerError("New API 返回密钥用量查询错误", token))
		}
		if !usage.Code {
			message := firstNonEmpty(usage.Message, "New API 返回密钥用量查询错误")
			return errResult(authID, label, providerBusinessError("", message, token))
		}
		if !usage.HasQuotaFields {
			return errResult(authID, label, invalidResponseError("New API 未返回密钥额度字段"))
		}
		var status newAPIStatusResp
		statusURL, _ := serviceEndpoint(n.BaseURL, "/api/status")
		_ = getJSONWithHeaders(statusURL, proxyURL, nil, &status)
		return parseNewAPITokenUsage(authID, usage, status)
	} else if !isHTTPStatusError(err, 404, 405) {
		return errResult(authID, label, err)
	}
	return n.fetchLegacyBilling(authID, token, proxyURL)
}

func parseNewAPITokenUsage(authID string, response newAPITokenUsageResp, status newAPIStatusResp) balance.Result {
	label := balance.ProviderLabel[balance.ProviderNewAPI]
	convert, unit := newAPIQuotaConverter(status)
	total := convert(response.Data.TotalGranted)
	used := convert(response.Data.TotalUsed)
	remaining := convert(response.Data.TotalAvailable)
	if _, totalOK := numberValue(total); !totalOK {
		return errResult(authID, label, invalidResponseError("New API 额度换算结果超出可表示范围"))
	}
	if _, usedOK := numberValue(used); !usedOK {
		return errResult(authID, label, invalidResponseError("New API 用量换算结果超出可表示范围"))
	}
	if _, remainingOK := numberValue(remaining); !remainingOK {
		return errResult(authID, label, invalidResponseError("New API 剩余额度换算结果超出可表示范围"))
	}
	showUsedWhenUnlimited := response.Data.UnlimitedQuota && (response.HasUsedField || response.Data.TotalUsed != 0)
	window := balance.QuotaWindow{
		Group:                 "密钥额度",
		Label:                 "总额度",
		Used:                  used,
		Total:                 total,
		Remaining:             remaining,
		Unit:                  unit,
		Unlimited:             response.Data.UnlimitedQuota,
		ShowUsedWhenUnlimited: showUsedWhenUnlimited,
		AggregationScope:      "key",
		AggregationKey:        "newapi:key-total",
	}
	if window.Unlimited {
		window.Total = 0
		window.Remaining = 0
	}
	if !window.Unlimited && total > 0 {
		window.UsedPercent = percentFromValues(used, total)
		window.RemainingPercent = clampPercent(100 - window.UsedPercent)
		window.CapacityPercent = 100
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
		if showUsedWhenUnlimited {
			r.QuotaDisplay = fmt.Sprintf("不限量，已用 %.4f %s", used, unit)
		} else {
			r.QuotaDisplay = "不限量"
		}
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
	displayType := strings.ToUpper(strings.TrimSpace(data.QuotaDisplayType))
	if displayType == "" && data.DisplayInCurrency != nil && !*data.DisplayInCurrency {
		return func(value float64) float64 { return value }, "内部额度"
	}
	switch displayType {
	case "TOKENS":
		return func(value float64) float64 { return value }, "令牌"
	case "CNY":
		rate := data.USDExchangeRate
		if rate <= 0 {
			return func(value float64) float64 { return value }, "内部额度"
		}
		return func(value float64) float64 { return value / data.QuotaPerUnit * rate }, "CNY"
	case "CUSTOM":
		rate := data.CustomCurrencyExchangeRate
		if rate <= 0 {
			return func(value float64) float64 { return value }, "内部额度"
		}
		unit := firstNonEmpty(data.CustomCurrencySymbol, "自定义币种")
		return func(value float64) float64 { return value / data.QuotaPerUnit * rate }, unit
	case "", "USD":
		return func(value float64) float64 { return value / data.QuotaPerUnit }, "USD"
	default:
		return func(value float64) float64 { return value }, "内部额度"
	}
}

func isHTTPStatusError(err error, statuses ...int) bool {
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		return false
	}
	for _, status := range statuses {
		if providerErr.HTTPStatus == status {
			return true
		}
	}
	return false
}

func (n NewAPI) fetchLegacyBilling(authID, token, proxyURL string) balance.Result {
	label := balance.ProviderLabel[balance.ProviderNewAPI]
	subscriptionURL, err := serviceEndpoint(n.BaseURL, "/v1/dashboard/billing/subscription")
	if err != nil {
		return errResult(authID, label, err)
	}
	usageURL, err := serviceEndpoint(n.BaseURL, "/v1/dashboard/billing/usage")
	if err != nil {
		return errResult(authID, label, err)
	}
	var subscription newAPISubscriptionResp
	if err := getJSON(subscriptionURL, token, proxyURL, &subscription); err != nil {
		return errResult(authID, label, err)
	}
	if subscription.Error != nil {
		return errResult(authID, label, subscription.Error.providerError("New API 返回订阅查询错误", token))
	}
	if !subscription.HasLimitField {
		return errResult(authID, label, invalidResponseError("New API 旧版订阅接口未返回额度字段"))
	}
	var usage newAPIBillingUsageResp
	if err := getJSON(usageURL, token, proxyURL, &usage); err != nil {
		return errResult(authID, label, err)
	}
	if usage.Error != nil {
		return errResult(authID, label, usage.Error.providerError("New API 返回用量查询错误", token))
	}
	if !usage.HasTotalUsage {
		return errResult(authID, label, invalidResponseError("New API 旧版用量接口未返回 total_usage"))
	}
	total := subscription.HardLimit
	if total <= 0 {
		total = subscription.SystemHardLimit
	}
	if total <= 0 {
		total = subscription.SoftLimit
	}
	used := usage.TotalUsage / 100
	unlimited := total == 100000000
	remaining := maxFloat(total-used, 0)
	if _, totalOK := numberValue(total); !totalOK {
		return errResult(authID, label, invalidResponseError("New API 旧版额度超出可表示范围"))
	}
	if _, usedOK := numberValue(used); !usedOK {
		return errResult(authID, label, invalidResponseError("New API 旧版用量超出可表示范围"))
	}
	if _, remainingOK := numberValue(remaining); !remainingOK {
		return errResult(authID, label, invalidResponseError("New API 旧版剩余额度超出可表示范围"))
	}
	if unlimited {
		total = 0
		remaining = 0
	}
	window := balance.QuotaWindow{
		Group:                 "密钥额度",
		Label:                 "总额度（兼容模式）",
		Used:                  used,
		Total:                 total,
		Remaining:             remaining,
		Unit:                  "站点额度",
		UsedPercent:           percentFromValues(used, total),
		RemainingPercent:      clampPercent(100 - percentFromValues(used, total)),
		Unlimited:             unlimited,
		ShowUsedWhenUnlimited: unlimited,
		AggregationScope:      "unknown",
		AggregationKey:        "newapi:legacy-total",
	}
	if subscription.AccessUntil > 0 {
		window.ResetAt = formatUnixTimestamp(subscription.AccessUntil)
	}
	if !unlimited && total > 0 {
		window.CapacityPercent = 100
	}
	quotaDisplay := fmt.Sprintf("剩余 %.4f / %.4f 站点额度", remaining, total)
	if unlimited {
		quotaDisplay = fmt.Sprintf("不限量，已用 %.4f 站点额度", used)
	}
	r := balance.Result{
		Provider:     label,
		AuthID:       authID,
		QuotaDisplay: quotaDisplay,
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
