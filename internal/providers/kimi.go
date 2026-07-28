package providers

import (
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
	Code    int    `json:"code"`
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Scode   string `json:"scode"`
	Data    struct {
		AvailableBalance float64 `json:"available_balance"`
		VoucherBalance   float64 `json:"voucher_balance"`
		CashBalance      float64 `json:"cash_balance"`
	} `json:"data"`
}

func (k KimiAPI) Fetch(authID, token, proxyURL string) balance.Result {
	label := balance.ProviderLabel[balance.ProviderKimiAPI]
	endpoint := "https://api.moonshot.ai/v1/users/me/balance"
	if strings.TrimSpace(k.BaseURL) != "" {
		if derived, err := serviceEndpoint(k.BaseURL, "/v1/users/me/balance"); err == nil {
			endpoint = derived
		} else {
			return errResult(authID, label, err.Error())
		}
	}
	var resp kimiBalanceResp
	if err := getJSON(endpoint, token, proxyURL, &resp); err != nil {
		return errResult(authID, label, err.Error())
	}
	if resp.Code != 0 || !resp.Status {
		message := firstNonEmpty(resp.Message, resp.Scode)
		if message == "" {
			message = fmt.Sprintf("余额接口返回业务错误（状态 %t，代码 %d）", resp.Status, resp.Code)
		}
		return errResult(authID, label, message)
	}
	return balance.Result{
		Provider:     label,
		AuthID:       authID,
		QuotaDisplay: fmt.Sprintf("可用 %.4f 元", resp.Data.AvailableBalance),
		Extra: map[string]string{
			"现金余额": fmt.Sprintf("%.4f 元", resp.Data.CashBalance),
			"赠金余额": fmt.Sprintf("%.4f 元", resp.Data.VoucherBalance),
		},
		FetchedAt: time.Now(),
	}
}

// KimiCode queries the official Kimi Coding Plan usage endpoint. The payload
// is top-level (usage/limits/boosterWallet), not wrapped in a data object.
// Product docs: https://www.kimi.com/code/docs/en/kimi-code/membership.html
// CLI parser: https://github.com/MoonshotAI/kimi-code/blob/main/packages/oauth/src/managed-usage.ts
type KimiCode struct{}

type kimiUsageResp struct {
	Usage         map[string]any   `json:"usage"`
	Limits        []map[string]any `json:"limits"`
	BoosterWallet map[string]any   `json:"boosterWallet"`
}

func (KimiCode) Fetch(authID, token, proxyURL string) balance.Result {
	label := balance.ProviderLabel[balance.ProviderKimiCode]
	var resp kimiUsageResp
	if err := getJSON("https://api.kimi.com/coding/v1/usages", token, proxyURL, &resp); err != nil {
		return errResult(authID, label, err.Error())
	}
	return parseKimiUsage(authID, resp)
}

func parseKimiUsage(authID string, resp kimiUsageResp) balance.Result {
	label := balance.ProviderLabel[balance.ProviderKimiCode]
	r := balance.Result{
		Provider:  label,
		AuthID:    authID,
		FetchedAt: time.Now(),
		Extra:     map[string]string{},
	}

	if window, ok := kimiQuotaWindow(resp.Usage, "每周配额"); ok {
		r.QuotaWindows = append(r.QuotaWindows, window)
		applyPrimaryWindow(&r, window)
	}
	for index, item := range resp.Limits {
		detail := item
		if nested, ok := item["detail"].(map[string]any); ok {
			detail = nested
		}
		fallback := kimiWindowLabel(item, detail, index)
		if window, ok := kimiQuotaWindow(detail, fallback); ok {
			r.QuotaWindows = append(r.QuotaWindows, window)
		}
	}
	parseKimiBoosterWallet(&r, resp.BoosterWallet)

	if len(r.Extra) == 0 {
		r.Extra = nil
	}
	if len(r.QuotaWindows) == 0 && len(r.Extra) == 0 {
		return errResult(authID, label, "官方接口未返回可识别的配额明细")
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
	if !usedOK && remainingOK && totalOK {
		used = maxFloat(total-remaining, 0)
		usedOK = true
	}
	if !remainingOK && usedOK && totalOK {
		remaining = maxFloat(total-used, 0)
		remainingOK = true
	}
	if !totalOK && !usedOK && !remainingOK {
		return balance.QuotaWindow{}, false
	}
	used = maxFloat(used, 0)
	remaining = maxFloat(remaining, 0)
	if total > 0 {
		used = minFloat(used, total)
		remaining = minFloat(remaining, total)
	}
	usedPercent := percentFromValues(used, total)
	remainingPercent := 0.0
	if total > 0 {
		remainingPercent = clampPercent(100 - usedPercent)
	}

	window := balance.QuotaWindow{
		Label:            localizedQuotaLabel(firstNonEmpty(firstString(values, "name", "title"), fallbackLabel)),
		UsedPercent:      usedPercent,
		RemainingPercent: remainingPercent,
		CapacityPercent:  100,
		AggregationScope: "key",
	}
	window.AggregationKey = "kimi:" + strings.ToLower(strings.TrimSpace(window.Label))
	// Kimi documents these values as shared membership quota rather than a
	// literal request allowance. Its official CLI likewise renders used/limit
	// only as a percentage, so exposing the raw ratio as "calls" is misleading.
	window.ResetAt = firstString(values, "reset_at", "resetAt", "reset_time", "resetTime")
	if resetIn, ok := firstNumber(values, "reset_in", "resetIn", "ttl", "window"); ok && resetIn > 0 {
		window.ResetInSeconds = int64(resetIn)
	}
	return window, true
}

func kimiWindowLabel(item, detail map[string]any, index int) string {
	if label := firstNonEmpty(firstString(item, "name", "title", "scope"), firstString(detail, "name", "title", "scope")); label != "" {
		return localizedQuotaLabel(label)
	}
	window, _ := item["window"].(map[string]any)
	duration, ok := firstNumber(window, "duration")
	if !ok {
		duration, ok = firstNumber(item, "duration")
	}
	if !ok {
		duration, ok = firstNumber(detail, "duration")
	}
	unit := strings.ToUpper(firstNonEmpty(firstString(window, "timeUnit"), firstString(item, "timeUnit"), firstString(detail, "timeUnit")))
	if ok && duration > 0 {
		seconds := int64(duration)
		switch {
		case strings.Contains(unit, "MINUTE"):
			seconds *= 60
		case strings.Contains(unit, "HOUR"):
			seconds *= 3600
		case strings.Contains(unit, "DAY"):
			seconds *= 86400
		}
		return durationWindowLabel(seconds)
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
			AggregationScope: "key",
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
			Status: "不限量", AggregationScope: "key", AggregationKey: "kimi:booster-monthly",
		})
	} else if enabled {
		result.Extra["每月付费上限"] = "已启用"
		result.QuotaWindows = append(result.QuotaWindows, balance.QuotaWindow{
			Group: "加量包", Label: "每月付费上限", Unit: currency, Unknown: true,
			Status: "接口未返回额度数值", AggregationScope: "key", AggregationKey: "kimi:booster-monthly",
		})
	} else if enabledPresent {
		result.Extra["每月付费上限"] = "未启用"
		result.QuotaWindows = append(result.QuotaWindows, balance.QuotaWindow{
			Group: "加量包", Label: "每月付费上限", Unit: currency, Unavailable: true,
			Status: "未启用", AggregationScope: "key", AggregationKey: "kimi:booster-monthly",
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
		result.TokensTotal = int64(window.Total)
		result.TokensUsed = int64(window.Used)
		result.TokensRemaining = int64(window.Remaining)
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
