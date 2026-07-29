package providers

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Hamster-Prime/balance-query/internal/balance"
)

// KimiAPI queries the official Moonshot/Kimi pay-as-you-go API balance.
// Docs: https://platform.moonshot.cn/docs/api/balance
type KimiAPI struct {
	BaseURL string
}

type kimiBalanceResp struct {
	Code                int    `json:"code"`
	Status              bool   `json:"status"`
	Message             string `json:"message"`
	Scode               string `json:"scode"`
	HasAvailableBalance bool   `json:"-"`
	HasVoucherBalance   bool   `json:"-"`
	HasCashBalance      bool   `json:"-"`
	Data                struct {
		AvailableBalance float64 `json:"available_balance"`
		VoucherBalance   float64 `json:"voucher_balance"`
		CashBalance      float64 `json:"cash_balance"`
	} `json:"data"`
}

func (response *kimiBalanceResp) UnmarshalJSON(data []byte) error {
	type wireResponse kimiBalanceResp
	var decoded wireResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*response = kimiBalanceResp(decoded)
	var envelope struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return err
	}
	if raw, exists := envelope.Data["available_balance"]; exists && validJSONNumber(raw) {
		response.HasAvailableBalance = true
	}
	if raw, exists := envelope.Data["voucher_balance"]; exists && validJSONNumber(raw) {
		response.HasVoucherBalance = true
	}
	if raw, exists := envelope.Data["cash_balance"]; exists && validJSONNumber(raw) {
		response.HasCashBalance = true
	}
	return nil
}

func (k KimiAPI) Fetch(authID, token, proxyURL string) balance.Result {
	label := balance.ProviderLabel[balance.ProviderKimiAPI]
	endpoint := "https://api.moonshot.ai/v1/users/me/balance"
	if strings.TrimSpace(k.BaseURL) != "" {
		if derived, err := serviceEndpoint(k.BaseURL, "/v1/users/me/balance"); err == nil {
			endpoint = derived
		} else {
			return errResult(authID, label, err)
		}
	}
	var resp kimiBalanceResp
	if err := getJSON(endpoint, token, proxyURL, &resp); err != nil {
		return errResult(authID, label, err)
	}
	if resp.Code != 0 || !resp.Status {
		message := firstNonEmpty(resp.Message, resp.Scode)
		if message == "" {
			message = fmt.Sprintf("余额接口返回业务错误（状态 %t，代码 %d）", resp.Status, resp.Code)
		}
		businessCode := firstNonEmpty(resp.Scode, fmt.Sprintf("%d", resp.Code))
		return errResult(authID, label, providerBusinessError(businessCode, message, token))
	}
	if !resp.HasAvailableBalance || !resp.HasVoucherBalance || !resp.HasCashBalance {
		return errResult(authID, label, invalidResponseError("Kimi 余额接口未返回完整余额字段"))
	}
	return balance.Result{
		Provider:         label,
		AuthID:           authID,
		BalanceAmount:    resp.Data.AvailableBalance,
		BalanceCurrency:  "CNY",
		HasBalanceAmount: true,
		BalanceScope:     "account",
		QuotaDisplay:     fmt.Sprintf("可用 %.4f 元", resp.Data.AvailableBalance),
		Extra: map[string]string{
			"现金余额": fmt.Sprintf("%.4f 元", resp.Data.CashBalance),
			"赠金余额": fmt.Sprintf("%.4f 元", resp.Data.VoucherBalance),
		},
		FetchedAt: time.Now(),
	}
}

// KimiCode queries the official Kimi Coding Plan usage endpoint. Current
// responses are top-level, while older/proxied responses may wrap the same
// fields in data; both shapes are accepted.
// Product docs: https://www.kimi.com/code/docs/en/kimi-code/membership.html
// CLI parser: https://github.com/MoonshotAI/kimi-code/blob/main/packages/oauth/src/managed-usage.ts
type KimiCode struct{}

type kimiUsageResp struct {
	Usage         map[string]any   `json:"usage"`
	Limits        []map[string]any `json:"limits"`
	BoosterWallet map[string]any   `json:"boosterWallet"`
	TotalQuota    any              `json:"totalQuota"`
	Data          *struct {
		Usage         map[string]any   `json:"usage"`
		Limits        []map[string]any `json:"limits"`
		BoosterWallet map[string]any   `json:"boosterWallet"`
		TotalQuota    any              `json:"totalQuota"`
	} `json:"data,omitempty"`
}

func (KimiCode) Fetch(authID, token, proxyURL string) balance.Result {
	label := balance.ProviderLabel[balance.ProviderKimiCode]
	var resp kimiUsageResp
	if err := getJSON("https://api.kimi.com/coding/v1/usages", token, proxyURL, &resp); err != nil {
		return errResult(authID, label, err)
	}
	return parseKimiUsage(authID, normalizeKimiUsage(resp))
}

func normalizeKimiUsage(resp kimiUsageResp) kimiUsageResp {
	if resp.Data == nil {
		return resp
	}
	if !hasKimiQuotaNumber(resp.Usage) && hasKimiQuotaNumber(resp.Data.Usage) {
		resp.Usage = resp.Data.Usage
	}
	if !hasKimiLimitPayload(resp.Limits) && hasKimiLimitPayload(resp.Data.Limits) {
		resp.Limits = resp.Data.Limits
	}
	if !hasKimiBoosterPayload(resp.BoosterWallet) && hasKimiBoosterPayload(resp.Data.BoosterWallet) {
		resp.BoosterWallet = resp.Data.BoosterWallet
	}
	if !hasKimiTotalQuota(resp.TotalQuota) && hasKimiTotalQuota(resp.Data.TotalQuota) {
		resp.TotalQuota = resp.Data.TotalQuota
	}
	resp.Data = nil
	return resp
}

func hasKimiUsagePayload(resp kimiUsageResp) bool {
	if hasKimiQuotaNumber(resp.Usage) {
		return true
	}
	if hasKimiTotalQuota(resp.TotalQuota) {
		return true
	}
	if hasKimiLimitPayload(resp.Limits) {
		return true
	}
	return hasKimiBoosterPayload(resp.BoosterWallet)
}

func hasKimiQuotaNumber(values map[string]any) bool {
	_, totalOK := firstNumber(values, "limit")
	_, usedOK := firstNumber(values, "used")
	_, remainingOK := firstNumber(values, "remaining")
	return totalOK || usedOK || remainingOK
}

func hasKimiTotalQuota(value any) bool {
	values, ok := value.(map[string]any)
	return ok && hasKimiQuotaNumber(values)
}

func hasKimiLimitPayload(limits []map[string]any) bool {
	for _, item := range limits {
		detail := item
		if nested, ok := item["detail"].(map[string]any); ok {
			detail = nested
		}
		if hasKimiQuotaNumber(detail) {
			return true
		}
	}
	return false
}

func hasKimiBoosterPayload(wallet map[string]any) bool {
	balanceValues, ok := wallet["balance"].(map[string]any)
	if !ok || !strings.EqualFold(firstString(balanceValues, "type"), "BOOSTER") {
		return false
	}
	total, totalOK := firstNumber(balanceValues, "amount")
	return totalOK && total > 0
}

func parseKimiUsage(authID string, resp kimiUsageResp) balance.Result {
	label := balance.ProviderLabel[balance.ProviderKimiCode]
	if !hasKimiUsagePayload(resp) {
		return errResult(authID, label, invalidResponseError("Kimi Code 官方接口未返回可识别的配额字段"))
	}
	r := balance.Result{
		Provider:  label,
		AuthID:    authID,
		FetchedAt: time.Now(),
		Extra:     map[string]string{},
	}

	weekly, ok := kimiQuotaWindow(resp.Usage, "每周配额")
	if !ok {
		weekly = balance.QuotaWindow{
			Label:            "每周配额",
			Unknown:          true,
			Status:           "接口未返回周配额，无法判断账户是否可用",
			AggregationScope: "account",
			AggregationKey:   "kimi:每周配额",
		}
	}
	weekly.Group = "订阅配额"
	r.QuotaWindows = append(r.QuotaWindows, weekly)
	applyPrimaryWindow(&r, weekly)
	if resp.TotalQuota != nil {
		totalWindow := balance.QuotaWindow{
			Group:            "订阅配额",
			Label:            "会员总额度",
			Unknown:          true,
			Status:           "接口返回的会员总额度格式无法识别",
			AggregationScope: "account",
			AggregationKey:   "kimi:total-quota",
		}
		if values, ok := resp.TotalQuota.(map[string]any); ok {
			if parsed, recognized := kimiQuotaWindow(values, "会员总额度"); recognized {
				totalWindow = parsed
				totalWindow.Group = "订阅配额"
				totalWindow.Label = "会员总额度"
				totalWindow.AggregationKey = "kimi:total-quota"
			}
		}
		r.QuotaWindows = append(r.QuotaWindows, totalWindow)
	}
	for index, item := range resp.Limits {
		detail := item
		if nested, ok := item["detail"].(map[string]any); ok {
			detail = nested
		}
		fallback := kimiWindowLabel(item, detail, index)
		if window, ok := kimiQuotaWindow(detail, fallback); ok {
			window.Group = "滚动限流"
			r.QuotaWindows = append(r.QuotaWindows, window)
		}
	}
	parseKimiBoosterWallet(&r, resp.BoosterWallet)

	if len(r.Extra) == 0 {
		r.Extra = nil
	}
	if len(r.QuotaWindows) == 0 && len(r.Extra) == 0 {
		return errResult(authID, label, invalidResponseError("官方接口未返回可识别的配额明细"))
	}
	return r
}

func kimiQuotaWindow(values map[string]any, fallbackLabel string) (balance.QuotaWindow, bool) {
	if len(values) == 0 {
		return balance.QuotaWindow{}, false
	}
	total, totalOK := firstNumber(values, "limit")
	used, usedOK := firstNumber(values, "used")
	remaining, remainingOK := firstNumber(values, "remaining")
	if !totalOK && !usedOK && !remainingOK {
		window := kimiQuotaWindowBase(values, fallbackLabel)
		window.Unknown = true
		window.Status = "接口未返回配额用量"
		return window, true
	}
	window := kimiQuotaWindowBase(values, fallbackLabel)
	if !totalOK || total <= 0 {
		window.Unknown = true
		window.Status = "接口未返回有效配额上限"
		return window, true
	}
	if !usedOK && !remainingOK {
		window.Unknown = true
		window.Status = "接口未返回已用或剩余额度"
		return window, true
	}
	used = maxFloat(used, 0)
	remaining = maxFloat(remaining, 0)
	used = minFloat(used, total)
	remaining = minFloat(remaining, total)
	resolvedRemaining := remaining
	inconsistent := false
	if usedOK {
		remainingFromUsed := maxFloat(total-used, 0)
		if remainingOK {
			inconsistent = math.Abs(remainingFromUsed-remaining) > math.Max(0.01, total*0.001)
			resolvedRemaining = minFloat(remaining, remainingFromUsed)
		} else {
			resolvedRemaining = remainingFromUsed
		}
	}
	window.RemainingPercent = clampPercent(percentFromValues(resolvedRemaining, total))
	window.UsedPercent = clampPercent(100 - window.RemainingPercent)
	window.CapacityPercent = 100
	if window.RemainingPercent <= 0 {
		window.Status = "已用尽"
	} else if inconsistent {
		window.Status = "接口额度字段不一致，已按较低剩余额度显示"
	}
	return window, true
}

func kimiQuotaWindowBase(values map[string]any, fallbackLabel string) balance.QuotaWindow {
	window := balance.QuotaWindow{
		Label:            localizedQuotaLabel(firstNonEmpty(firstString(values, "name", "title"), fallbackLabel)),
		AggregationScope: "account",
	}
	window.AggregationKey = "kimi:" + strings.ToLower(strings.TrimSpace(window.Label))
	window.ResetAt = normalizedTimestampForDisplay(firstValue(values, "reset_at", "resetAt", "reset_time", "resetTime"))
	if resetIn, ok := int64Value(firstValue(values, "reset_in", "resetIn", "ttl", "window")); ok && resetIn > 0 {
		window.ResetInSeconds = resetIn
	}
	return window
}

func kimiWindowLabel(item, detail map[string]any, index int) string {
	if label := firstNonEmpty(firstString(item, "name", "title", "scope"), firstString(detail, "name", "title", "scope")); label != "" {
		return localizedQuotaLabel(label)
	}
	window, _ := item["window"].(map[string]any)
	durationValue := firstValue(window, "duration")
	if durationValue == nil {
		durationValue = firstValue(item, "duration")
	}
	if durationValue == nil {
		durationValue = firstValue(detail, "duration")
	}
	duration, ok := int64Value(durationValue)
	unit := strings.ToUpper(firstNonEmpty(firstString(window, "timeUnit"), firstString(item, "timeUnit"), firstString(detail, "timeUnit")))
	if ok && duration > 0 {
		multiplier := int64(1)
		switch {
		case strings.Contains(unit, "MINUTE"):
			multiplier = 60
		case strings.Contains(unit, "HOUR"):
			multiplier = 3600
		case strings.Contains(unit, "DAY"):
			multiplier = 86400
		}
		if duration <= math.MaxInt64/multiplier {
			return durationWindowLabel(duration * multiplier)
		}
	}
	return fmt.Sprintf("其他配额 %d", index+1)
}

func parseKimiBoosterWallet(result *balance.Result, wallet map[string]any) {
	if len(wallet) == 0 {
		return
	}
	balanceValues, _ := wallet["balance"].(map[string]any)
	if !strings.EqualFold(firstString(balanceValues, "type"), "BOOSTER") {
		return
	}
	totalRaw, totalOK := firstNumber(balanceValues, "amount")
	remainingRaw, remainingOK := firstNumber(balanceValues, "amountLeft")
	if !totalOK || totalRaw <= 0 {
		return
	}
	// The API uses fixed-point cents: 1,000,000 units equal one cent. Match
	// Kimi's official CLI by rounding to whole cents and preserving a minimum
	// positive amount of one cent.
	total := kimiFixedPointDollars(totalRaw)
	remaining := 0.0
	if remainingOK {
		remaining = kimiFixedPointDollars(remainingRaw)
	}
	currency := "USD"
	monthlyLimit, _ := wallet["monthlyChargeLimit"].(map[string]any)
	monthlyUsed, _ := wallet["monthlyUsed"].(map[string]any)
	if candidate := firstNonEmpty(firstString(monthlyLimit, "currency"), firstString(monthlyUsed, "currency")); candidate != "" {
		currency = candidate
	}
	result.Extra["加量包余额"] = fmt.Sprintf("%.2f / %.2f %s", remaining, total, currency)

	limitCents, limitOK := firstNumber(monthlyLimit, "priceInCents")
	usedCents, usedOK := firstNumber(monthlyUsed, "priceInCents")
	enabled, enabledPresent := wallet["monthlyChargeLimitEnabled"].(bool)
	if enabled && limitOK && limitCents > 0 {
		used := 0.0
		if usedOK {
			used = math.Trunc(usedCents) / 100
		}
		totalLimit := math.Trunc(limitCents) / 100
		result.QuotaWindows = append(result.QuotaWindows, balance.QuotaWindow{
			Group:            "加量包",
			Label:            "每月付费上限",
			Used:             used,
			Total:            totalLimit,
			Remaining:        maxFloat(totalLimit-used, 0),
			Unit:             currency,
			UsedPercent:      percentFromValues(used, totalLimit),
			RemainingPercent: clampPercent(100 - percentFromValues(used, totalLimit)),
			CapacityPercent:  100,
			AggregationScope: "account",
			AggregationKey:   "kimi:booster-monthly",
		})
	} else if enabled && limitOK && limitCents == 0 {
		used := 0.0
		if usedOK {
			used = math.Trunc(usedCents) / 100
		}
		result.Extra["每月付费上限"] = "不限量"
		result.QuotaWindows = append(result.QuotaWindows, balance.QuotaWindow{
			Group: "加量包", Label: "每月付费上限", Unit: currency, Used: used,
			Unlimited: true, ShowUsedWhenUnlimited: usedOK,
			Status: "不限量", AggregationScope: "account", AggregationKey: "kimi:booster-monthly",
		})
	} else if enabled {
		result.Extra["每月付费上限"] = "已启用"
		result.QuotaWindows = append(result.QuotaWindows, balance.QuotaWindow{
			Group: "加量包", Label: "每月付费上限", Unit: currency, Unknown: true,
			Status: "接口未返回额度数值", AggregationScope: "account", AggregationKey: "kimi:booster-monthly",
		})
	} else if enabledPresent {
		result.Extra["每月付费上限"] = "未启用"
		result.QuotaWindows = append(result.QuotaWindows, balance.QuotaWindow{
			Group: "加量包", Label: "每月付费上限", Unit: currency, Unavailable: true,
			Status: "未启用", AggregationScope: "account", AggregationKey: "kimi:booster-monthly",
		})
	}
}

func kimiFixedPointDollars(raw float64) float64 {
	cents := raw / 1_000_000
	if cents > 0 && cents < 1 {
		cents = 1
	} else {
		cents = math.Round(cents)
	}
	return cents / 100
}

func applyPrimaryWindow(result *balance.Result, window balance.QuotaWindow) {
	if result == nil {
		return
	}
	result.ResetAt = window.ResetAt
	if window.Total > 0 {
		if total, totalOK := int64Value(window.Total); totalOK {
			if used, usedOK := int64Value(window.Used); usedOK {
				if remaining, remainingOK := int64Value(window.Remaining); remainingOK {
					result.TokensTotal = total
					result.TokensUsed = used
					result.TokensRemaining = remaining
				}
			}
		}
		result.QuotaDisplay = fmt.Sprintf("%s：剩余 %s / %s %s", window.Label,
			formatQuotaNumber(window.Remaining), formatQuotaNumber(window.Total), window.Unit)
	} else if window.RemainingPercent > 0 || window.UsedPercent > 0 {
		result.QuotaDisplay = fmt.Sprintf("%s：剩余 %s%%", window.Label,
			formatQuotaNumber(window.RemainingPercent))
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func maxFloat(left, right float64) float64 {
	if left > right {
		return left
	}
	return right
}

func minFloat(left, right float64) float64 {
	if left < right {
		return left
	}
	return right
}
