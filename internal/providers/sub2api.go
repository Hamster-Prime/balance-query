package providers

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Hamster-Prime/balance-query/internal/balance"
)

// Sub2API queries the official API-key accounting endpoint GET /v1/usage.
// The response is top-level and has quota_limited and unrestricted modes.
type Sub2API struct {
	BaseURL string
}

func (s Sub2API) Fetch(authID, token, proxyURL string) balance.Result {
	label := balance.ProviderLabel[balance.ProviderSub2API]
	if strings.TrimSpace(s.BaseURL) == "" {
		return errResult(authID, label,
			newProviderError(balance.FailureInvalidConfig, "所选 AI 提供商没有配置接口地址", 0, "", nil))
	}
	endpoint, err := serviceEndpoint(s.BaseURL, "/v1/usage")
	if err != nil {
		return errResult(authID, label, err)
	}

	endpoint += "?days=30"
	var payload map[string]any
	if err := getSub2APIJSON(endpoint, token, proxyURL, &payload); err != nil {
		return errResult(authID, label, err)
	}
	r := parseSub2APIUsage(authID, payload, token)
	if r.Error != "" {
		return r
	}
	r.Provider = label
	r.AuthID = authID
	r.FetchedAt = time.Now()
	return r
}

func getSub2APIJSON(endpoint, token, proxyURL string, dest any) error {
	err := getJSON(endpoint, token, proxyURL, dest)
	if err == nil || !isHTTPStatusError(err, 401) || !isAuthenticationFailure(err) {
		return err
	}
	for _, header := range []string{"x-api-key", "x-goog-api-key"} {
		err = getJSONWithHeaders(endpoint, proxyURL, map[string]string{header: token}, dest)
		if err == nil || !isHTTPStatusError(err, 401) || !isAuthenticationFailure(err) {
			return err
		}
	}
	return err
}

func parseSub2APIUsage(authID string, payload map[string]any, secrets ...string) balance.Result {
	if failure := sub2APIBusinessFailure(payload, secrets...); failure != nil {
		return errResult(authID, balance.ProviderLabel[balance.ProviderSub2API], failure)
	}
	r := balance.Result{
		Provider:  balance.ProviderLabel[balance.ProviderSub2API],
		AuthID:    authID,
		FetchedAt: time.Now(),
		Extra:     map[string]string{},
	}
	mode := firstString(payload, "mode")
	r.Plan = firstString(payload, "planName")
	if r.Plan == "" {
		switch mode {
		case "quota_limited":
			r.Plan = "密钥独立额度"
		case "unrestricted":
			r.Plan = "账户额度"
		}
	}
	if valid, exists := payload["isValid"].(bool); exists {
		if valid {
			r.Extra["密钥状态"] = "有效"
		} else {
			r.Extra["密钥状态"] = "不可用"
		}
	}
	if status := firstString(payload, "status"); status != "" {
		r.Extra["状态详情"] = localizedSub2Status(status)
	}
	unit := firstNonEmpty(firstString(payload, "unit"), "USD")

	if quota, ok := payload["quota"].(map[string]any); ok {
		window := quotaWindowFromMap("密钥额度", "总额度", quota, firstNonEmpty(firstString(quota, "unit"), unit))
		window.AggregationScope = "key"
		window.AggregationKey = "sub2api:key-total"
		r.QuotaWindows = append(r.QuotaWindows, window)
		if !window.Unknown {
			applyPrimaryWindow(&r, window)
		}
	}

	if rawLimits, ok := payload["rate_limits"].([]any); ok {
		for _, raw := range rawLimits {
			limit, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			windowCode := strings.ToLower(firstString(limit, "window"))
			windowLabel := map[string]string{
				"5h": "5 小时额度", "1d": "每日额度", "7d": "7 天额度",
			}[windowCode]
			if windowLabel == "" {
				windowLabel = firstNonEmpty(windowCode, "限流额度")
			}
			window := quotaWindowFromMap("速率限制", windowLabel, limit, unit)
			window.AggregationScope = "key"
			window.AggregationKey = "sub2api:rate:" + windowCode
			window.ResetAt = normalizedTimestampForDisplay(firstValue(limit, "reset_at"))
			r.QuotaWindows = append(r.QuotaWindows, window)
		}
	}

	if subscription, ok := payload["subscription"].(map[string]any); ok {
		appendSubscriptionWindow := func(label, period, usedKey, limitKey string) {
			rawLimit, limitPresent := subscription[limitKey]
			if !limitPresent {
				return
			}
			used, usedOK := firstNumber(subscription, usedKey)
			total, totalOK := numberValue(rawLimit)
			window := balance.QuotaWindow{
				Group:            "订阅套餐",
				Label:            label,
				Unit:             unit,
				AggregationScope: "account",
				AggregationKey:   "sub2api:subscription:" + period,
			}
			if !totalOK {
				window.Unknown = true
				window.Status = "接口未返回有效套餐上限"
				r.QuotaWindows = append(r.QuotaWindows, window)
				return
			}
			if total == 0 {
				window.Unlimited = true
				window.Status = "不限量"
				r.QuotaWindows = append(r.QuotaWindows, window)
				return
			}
			if total < 0 {
				window.Unknown = true
				window.Status = "接口返回了无效的负数套餐上限"
				r.QuotaWindows = append(r.QuotaWindows, window)
				return
			}
			if !usedOK {
				window.Total = total
				window.Unknown = true
				window.Status = "接口未返回有效已用额度"
				r.QuotaWindows = append(r.QuotaWindows, window)
				return
			}
			used = minFloat(maxFloat(used, 0), total)
			window.Used = used
			window.Total = total
			window.Remaining = maxFloat(total-used, 0)
			window.UsedPercent = percentFromValues(used, total)
			window.RemainingPercent = clampPercent(100 - window.UsedPercent)
			window.CapacityPercent = 100
			if label == "每周额度" {
				// The API returns the beginning of the rolling weekly window,
				// not its reset timestamp. The service resets it seven days later.
				window.ResetAt = addToTimestamp(firstValue(subscription, "weekly_window_start"), 7*24*time.Hour)
			}
			r.QuotaWindows = append(r.QuotaWindows, window)
		}
		appendSubscriptionWindow("每日额度", "daily", "daily_usage_usd", "daily_limit_usd")
		appendSubscriptionWindow("每周额度", "weekly", "weekly_usage_usd", "weekly_limit_usd")
		appendSubscriptionWindow("每月额度", "monthly", "monthly_usage_usd", "monthly_limit_usd")
		if expires := normalizedTimestampForDisplay(firstValue(subscription, "expires_at")); expires != "" {
			r.ResetAt = expires
			r.Extra["套餐到期"] = expires
		}
	}

	if balanceValue, ok := firstNumber(payload, "balance"); ok {
		r.BalanceAmount = balanceValue
		r.BalanceCurrency = unit
		r.HasBalanceAmount = true
		r.BalanceScope = "account"
		if strings.EqualFold(unit, "USD") {
			r.BalanceUSD = balanceValue
			r.HasBalance = true
		}
		if r.QuotaDisplay == "" {
			r.QuotaDisplay = fmt.Sprintf("钱包余额 %.4f %s", balanceValue, unit)
		}
	}
	if remaining, ok := firstNumber(payload, "remaining"); ok && r.QuotaDisplay == "" {
		if remaining < 0 {
			r.QuotaDisplay = "套餐额度不设上限"
		} else {
			r.QuotaDisplay = fmt.Sprintf("当前可用 %.4f %s", remaining, unit)
		}
	}
	if expires := normalizedTimestampForDisplay(firstValue(payload, "expires_at")); expires != "" {
		r.ResetAt = expires
		r.Extra["密钥到期"] = expires
	}
	if days, ok := firstNumber(payload, "days_until_expiry"); ok {
		r.Extra["距离到期"] = fmt.Sprintf("%.0f 天", days)
	}
	appendSub2APIUsageDetails(&r, payload)
	appendSub2APIHistoryDetails(&r, payload)

	if len(r.Extra) == 0 {
		r.Extra = nil
	}
	if len(r.QuotaWindows) == 0 && r.QuotaDisplay == "" && len(r.Extra) == 0 {
		return errResult(authID, balance.ProviderLabel[balance.ProviderSub2API],
			invalidResponseError("Sub2API 未返回可识别的额度或用量明细"))
	}
	return r
}

func sub2APIBusinessFailure(payload map[string]any, secrets ...string) error {
	code, codePresent, codeSuccess := businessCodeValue(firstValue(payload, "code"))
	message := firstString(payload, "message", "msg")
	explicitFailure := codePresent && !codeSuccess
	if success, exists := payload["success"].(bool); exists && !success {
		explicitFailure = true
	}

	switch upstreamError := payload["error"].(type) {
	case string:
		if strings.TrimSpace(upstreamError) != "" {
			explicitFailure = true
			message = firstNonEmpty(message, upstreamError)
		}
	case map[string]any:
		nestedCode, nestedPresent, nestedSuccess := businessCodeValue(firstValue(upstreamError, "code", "type"))
		nestedMessage := firstString(upstreamError, "message", "msg", "detail", "reason")
		if nestedPresent && !nestedSuccess || nestedMessage != "" {
			explicitFailure = true
			if code == "" || codeSuccess {
				code = nestedCode
			}
			message = firstNonEmpty(message, nestedMessage)
		}
	}
	if !explicitFailure {
		return nil
	}
	message = firstNonEmpty(message, "Sub2API 返回业务错误 "+firstNonEmpty(code, "UNKNOWN"))
	return providerBusinessError(code, message, secrets...)
}

func appendSub2APIHistoryDetails(result *balance.Result, payload map[string]any) {
	if result == nil {
		return
	}
	if rawDaily, ok := payload["daily_usage"].([]any); ok {
		daily := make([]map[string]any, 0, len(rawDaily))
		var requests, tokens, actualCost float64
		for _, raw := range rawDaily {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			daily = append(daily, item)
			if value, ok := firstNumber(item, "requests"); ok {
				requests += value
			}
			if value, ok := firstNumber(item, "total_tokens"); ok {
				tokens += value
			}
			if value, ok := firstNumber(item, "actual_cost"); ok {
				actualCost += value
			}
		}
		if len(daily) > 0 {
			result.Extra["近 30 天请求"] = fmt.Sprintf("%.0f 次", requests)
			result.Extra["近 30 天令牌"] = fmt.Sprintf("%.0f", tokens)
			result.Extra["近 30 天实际成本"] = fmt.Sprintf("%.4f USD", actualCost)
			sort.SliceStable(daily, func(left, right int) bool {
				return firstString(daily[left], "date") > firstString(daily[right], "date")
			})
			for index, item := range daily {
				if index >= 7 {
					break
				}
				date := firstString(item, "date")
				if date == "" {
					continue
				}
				dayRequests, _ := firstNumber(item, "requests")
				dayTokens, _ := firstNumber(item, "total_tokens")
				dayCost, _ := firstNumber(item, "actual_cost")
				result.Extra[date+" 用量"] = fmt.Sprintf("%.0f 次请求 · %.0f 令牌 · %.4f USD", dayRequests, dayTokens, dayCost)
			}
		}
	}

	if rawModels, ok := payload["model_stats"].([]any); ok {
		models := make([]map[string]any, 0, len(rawModels))
		for _, raw := range rawModels {
			if item, ok := raw.(map[string]any); ok {
				models = append(models, item)
			}
		}
		sort.SliceStable(models, func(left, right int) bool {
			leftCost, _ := firstNumber(models[left], "actual_cost", "cost")
			rightCost, _ := firstNumber(models[right], "actual_cost", "cost")
			if leftCost != rightCost {
				return leftCost > rightCost
			}
			leftTokens, _ := firstNumber(models[left], "total_tokens")
			rightTokens, _ := firstNumber(models[right], "total_tokens")
			return leftTokens > rightTokens
		})
		for index, item := range models {
			if index >= 8 {
				break
			}
			model := firstString(item, "model")
			if model == "" {
				continue
			}
			requests, _ := firstNumber(item, "requests")
			tokens, _ := firstNumber(item, "total_tokens")
			cost, _ := firstNumber(item, "actual_cost", "cost")
			result.Extra["模型 "+model] = fmt.Sprintf("%.0f 次请求 · %.0f 令牌 · %.4f USD", requests, tokens, cost)
		}
	}
}

func localizedSub2Status(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active":
		return "正常"
	case "disabled":
		return "已禁用"
	case "quota_exhausted":
		return "额度已用尽"
	case "expired":
		return "已过期"
	default:
		return strings.TrimSpace(status)
	}
}

func quotaWindowFromMap(group, label string, values map[string]any, unit string) balance.QuotaWindow {
	total, totalOK := firstNumber(values, "limit", "total")
	used, usedOK := firstNumber(values, "used")
	remaining, remainingOK := firstNumber(values, "remaining")
	window := balance.QuotaWindow{
		Group: group,
		Label: label,
		Unit:  unit,
	}
	if !totalOK && !usedOK && !remainingOK {
		window.Unknown = true
		window.Status = "接口未返回额度数值"
		return window
	}
	if !totalOK || total <= 0 {
		window.Total = maxFloat(total, 0)
		if usedOK {
			window.Used = maxFloat(used, 0)
		}
		if remainingOK {
			window.Remaining = maxFloat(remaining, 0)
		}
		window.Unknown = true
		window.Status = "接口未返回有效额度上限"
		return window
	}
	if !usedOK && !remainingOK {
		window.Total = total
		window.Unknown = true
		window.Status = "接口未返回已用或剩余额度"
		return window
	}
	if usedOK {
		used = minFloat(maxFloat(used, 0), total)
	}
	if remainingOK {
		remaining = minFloat(maxFloat(remaining, 0), total)
	}
	inconsistent := false
	if usedOK && remainingOK {
		remainingFromUsed := total - used
		difference := remainingFromUsed - remaining
		if difference < 0 {
			difference = -difference
		}
		inconsistent = difference > maxFloat(0.01, total*0.001)
		remaining = minFloat(remaining, remainingFromUsed)
		used = total - remaining
	} else if remainingOK {
		used = total - remaining
	} else {
		remaining = total - used
	}
	usedPercent := percentFromValues(used, total)
	window.Used = used
	window.Total = total
	window.Remaining = remaining
	window.UsedPercent = usedPercent
	window.RemainingPercent = clampPercent(100 - usedPercent)
	window.CapacityPercent = 100
	if remaining <= 0 {
		window.Status = "已用尽"
	} else if inconsistent {
		window.Status = "接口额度字段不一致，已按较低剩余额度显示"
	}
	return window
}

func appendSub2APIUsageDetails(result *balance.Result, payload map[string]any) {
	usage, ok := payload["usage"].(map[string]any)
	if !ok {
		return
	}
	for _, item := range []struct {
		key   string
		label string
	}{
		{"today", "今日"},
		{"total", "累计"},
	} {
		period, ok := usage[item.key].(map[string]any)
		if !ok {
			continue
		}
		if value, ok := firstNumber(period, "requests"); ok {
			result.Extra[item.label+"请求"] = fmt.Sprintf("%.0f 次", value)
		}
		if value, ok := firstNumber(period, "total_tokens"); ok {
			result.Extra[item.label+"令牌"] = fmt.Sprintf("%.0f", value)
		}
		if value, ok := firstNumber(period, "input_tokens"); ok {
			result.Extra[item.label+"输入"] = fmt.Sprintf("%.0f 令牌", value)
		}
		if value, ok := firstNumber(period, "output_tokens"); ok {
			result.Extra[item.label+"输出"] = fmt.Sprintf("%.0f 令牌", value)
		}
		if value, ok := firstNumber(period, "actual_cost"); ok {
			result.Extra[item.label+"实际成本"] = fmt.Sprintf("%.4f USD", value)
		} else if value, ok := firstNumber(period, "cost"); ok {
			result.Extra[item.label+"成本"] = fmt.Sprintf("%.4f USD", value)
		}
	}
	if value, ok := firstNumber(usage, "rpm"); ok {
		result.Extra["当前 RPM"] = fmt.Sprintf("%.2f", value)
	}
	if value, ok := firstNumber(usage, "tpm"); ok {
		result.Extra["当前 TPM"] = fmt.Sprintf("%.2f", value)
	}
	if value, ok := firstNumber(usage, "average_duration_ms"); ok {
		result.Extra["平均响应耗时"] = fmt.Sprintf("%.0f ms", value)
	}
}
