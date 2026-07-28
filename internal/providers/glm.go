package providers

import (
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Hamster-Prime/balance-query/internal/balance"
)

// GLM Coding Plan quota endpoints use the token directly in Authorization,
// without the Bearer prefix.
type GLMZai struct{}
type GLMZhipu struct{}

type glmQuotaResp struct {
	Code    any    `json:"code"`
	Success bool   `json:"success"`
	Message string `json:"message"`
	Msg     string `json:"msg"`
	Data    struct {
		Level       string         `json:"level"`
		PlanName    string         `json:"planName"`
		Plan        string         `json:"plan"`
		PlanType    string         `json:"plan_type"`
		PackageName string         `json:"packageName"`
		Limits      []glmLimitItem `json:"limits"`
	} `json:"data"`
}

type glmDetailResp struct {
	Code    any            `json:"code"`
	Success bool           `json:"success"`
	Message string         `json:"message"`
	Msg     string         `json:"msg"`
	Data    map[string]any `json:"data"`
}

type glmLimitItem struct {
	Type          string   `json:"type"`
	Percentage    *float64 `json:"percentage"` // used percentage; nil means omitted
	Unit          int64    `json:"unit"`
	Number        int64    `json:"number"`
	CurrentValue  *float64 `json:"currentValue"`
	Remaining     *float64 `json:"remaining"`
	Total         float64  `json:"total"`
	Usage         float64  `json:"usage"`
	UsageDetails  any      `json:"usageDetails"`
	NextResetTime int64    `json:"nextResetTime"`
}

func fetchGLMQuota(authID, token, proxyURL, baseURL, label string) balance.Result {
	startTime, endTime := glmUsageRange(time.Now())
	query := "?startTime=" + url.QueryEscape(startTime) + "&endTime=" + url.QueryEscape(endTime)

	var resp glmQuotaResp
	var modelResp, toolResp glmDetailResp
	var quotaErr, modelErr, toolErr error
	var wait sync.WaitGroup
	wait.Add(3)
	go func() {
		defer wait.Done()
		quotaErr = fetchGLMEndpoint(baseURL+"/api/monitor/usage/quota/limit", token, proxyURL, &resp)
	}()
	go func() {
		defer wait.Done()
		modelErr = fetchGLMEndpoint(baseURL+"/api/monitor/usage/model-usage"+query, token, proxyURL, &modelResp)
	}()
	go func() {
		defer wait.Done()
		toolErr = fetchGLMEndpoint(baseURL+"/api/monitor/usage/tool-usage"+query, token, proxyURL, &toolResp)
	}()
	wait.Wait()
	if quotaErr != nil {
		return errResult(authID, label, quotaErr.Error())
	}
	if code, ok := numberValue(resp.Code); ok && code != 0 && code != 200 {
		return errResult(authID, label, firstNonEmpty(resp.Message, resp.Msg,
			fmt.Sprintf("GLM 配额接口返回业务错误 %.0f", code)))
	}
	if len(resp.Data.Limits) == 0 {
		message := firstNonEmpty(resp.Message, resp.Msg)
		if message == "" {
			message = "官方接口未返回 Coding Plan 配额数据"
		}
		return errResult(authID, label, message)
	}

	r := balance.Result{
		Provider: label,
		AuthID:   authID,
		Plan: firstNonEmpty(resp.Data.PlanName, resp.Data.PackageName,
			resp.Data.Plan, resp.Data.PlanType, resp.Data.Level),
		FetchedAt: time.Now(),
		Extra:     map[string]string{},
	}
	if r.Plan != "" {
		r.Extra["套餐名称"] = r.Plan
	}

	for _, limit := range resp.Data.Limits {
		window := glmQuotaWindow(limit)
		r.QuotaWindows = append(r.QuotaWindows, window)
		if r.QuotaDisplay == "" && limit.Type == "TOKENS_LIMIT" {
			applyPrimaryWindow(&r, window)
		}
		if limit.Type == "TIME_LIMIT" {
			for _, detail := range glmUsageDetails(limit.UsageDetails) {
				code := firstString(detail, "modelCode", "model_code", "name")
				usage, ok := firstNumber(detail, "usage", "currentValue", "count")
				if !ok || code == "" {
					continue
				}
				r.Extra[glmToolLabel(code)] = fmt.Sprintf("%.0f 次", usage)
			}
		}
	}
	if modelErr == nil && glmDetailSuccess(modelResp) {
		appendGLMModelDetails(&r, modelResp.Data)
	} else {
		r.Extra["24 小时模型统计"] = "暂不可用"
	}
	if toolErr == nil && glmDetailSuccess(toolResp) {
		appendGLMToolDetails(&r, toolResp.Data)
	} else {
		r.Extra["24 小时工具统计"] = "暂不可用"
	}
	if len(r.Extra) == 0 {
		r.Extra = nil
	}
	return r
}

func fetchGLMEndpoint(endpoint, token, proxyURL string, dest any) error {
	err := getJSONWithHeaders(endpoint, proxyURL, map[string]string{
		"Authorization":   token,
		"Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
	}, dest)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "http 401") {
		return err
	}
	return getJSONWithHeaders(endpoint, proxyURL, map[string]string{
		"Authorization":   "Bearer " + token,
		"Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
	}, dest)
}

func glmUsageRange(now time.Time) (string, string) {
	end := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 59, 59, 0, now.Location())
	startDay := end.AddDate(0, 0, -1)
	start := time.Date(startDay.Year(), startDay.Month(), startDay.Day(), now.Hour(), 0, 0, 0, now.Location())
	const layout = "2006-01-02 15:04:05"
	return start.Format(layout), end.Format(layout)
}

func glmDetailSuccess(response glmDetailResp) bool {
	if response.Success {
		return true
	}
	code, ok := numberValue(response.Code)
	return ok && (code == 0 || code == 200)
}

func appendGLMModelDetails(result *balance.Result, data map[string]any) {
	if result == nil || len(data) == 0 {
		return
	}
	totalUsage, _ := data["totalUsage"].(map[string]any)
	if calls, ok := firstNumber(totalUsage, "totalModelCallCount"); ok {
		result.Extra["24 小时模型调用"] = fmt.Sprintf("%.0f 次", calls)
	}
	if tokens, ok := firstNumber(totalUsage, "totalTokensUsage"); ok {
		result.Extra["24 小时令牌用量"] = fmt.Sprintf("%.0f", tokens)
	}
	summaries := firstSummaryList(data, totalUsage, "modelSummaryList")
	for _, summary := range summaries {
		name := firstString(summary, "modelName", "modelCode", "name")
		tokens, ok := firstNumber(summary, "totalTokens", "tokensUsage", "totalTokensUsage")
		if name == "" || !ok {
			continue
		}
		result.Extra["24 小时 "+name] = fmt.Sprintf("%.0f 令牌", tokens)
	}
}

func appendGLMToolDetails(result *balance.Result, data map[string]any) {
	if result == nil || len(data) == 0 {
		return
	}
	totalUsage, _ := data["totalUsage"].(map[string]any)
	for _, item := range []struct {
		key   string
		label string
	}{
		{"totalNetworkSearchCount", "24 小时联网搜索"},
		{"totalWebReadMcpCount", "24 小时网页读取"},
		{"totalZreadMcpCount", "24 小时 ZRead"},
		{"totalSearchMcpCount", "24 小时搜索 MCP"},
	} {
		if count, ok := firstNumber(totalUsage, item.key); ok {
			result.Extra[item.label] = fmt.Sprintf("%.0f 次", count)
		}
	}
	summaries := firstSummaryList(data, totalUsage, "toolSummaryList")
	for _, summary := range summaries {
		name := firstNonEmpty(firstString(summary, "toolNameI18n"), firstString(summary, "toolName"), firstString(summary, "toolCode"))
		count, ok := firstNumber(summary, "totalUsageCount", "usage")
		if name == "" || !ok {
			continue
		}
		result.Extra["24 小时 "+name] = fmt.Sprintf("%.0f 次", count)
	}
}

func firstSummaryList(data, totalUsage map[string]any, key string) []map[string]any {
	for _, source := range []map[string]any{data, totalUsage} {
		raw, ok := source[key].([]any)
		if !ok {
			continue
		}
		items := make([]map[string]any, 0, len(raw))
		for _, value := range raw {
			if item, ok := value.(map[string]any); ok {
				items = append(items, item)
			}
		}
		if len(items) > 0 {
			return items
		}
	}
	return nil
}

func glmQuotaWindow(limit glmLimitItem) balance.QuotaWindow {
	label := glmWindowLabel(limit)
	total := limit.Usage
	if total <= 0 {
		total = limit.Total
	}
	used := 0.0
	hasUsed := limit.CurrentValue != nil
	if hasUsed {
		used = maxFloat(*limit.CurrentValue, 0)
	}
	usedPercent := 0.0
	hasPercentage := limit.Percentage != nil
	if hasPercentage {
		usedPercent = clampPercent(*limit.Percentage)
	}
	if !hasUsed && total > 0 && hasPercentage {
		used = total * usedPercent / 100
		hasUsed = true
	}
	remaining := 0.0
	hasRemaining := limit.Remaining != nil
	if limit.Remaining != nil {
		remaining = maxFloat(*limit.Remaining, 0)
	} else if total > 0 && hasUsed {
		remaining = maxFloat(total-used, 0)
		hasRemaining = true
	}
	if !hasUsed && total > 0 && hasRemaining {
		used = maxFloat(total-remaining, 0)
		hasUsed = true
	}
	unit := "令牌"
	group := "模型额度"
	if limit.Type == "TIME_LIMIT" {
		unit = "次"
		group = "MCP 工具"
	}
	window := balance.QuotaWindow{
		Group:            group,
		Label:            label,
		Used:             used,
		Total:            total,
		Remaining:        remaining,
		Unit:             unit,
		ResetAt:          formatUnixTimestamp(limit.NextResetTime),
		AggregationScope: "key",
		AggregationKey:   fmt.Sprintf("glm:%s:%d:%d", limit.Type, limit.Unit, limit.Number),
	}
	if hasPercentage {
		window.UsedPercent = usedPercent
		window.RemainingPercent = clampPercent(100 - usedPercent)
		window.CapacityPercent = 100
	} else if total > 0 && (hasUsed || hasRemaining) {
		if hasRemaining {
			window.RemainingPercent = clampPercent(percentFromValues(remaining, total))
			window.UsedPercent = clampPercent(100 - window.RemainingPercent)
		} else {
			window.UsedPercent = percentFromValues(used, total)
			window.RemainingPercent = clampPercent(100 - window.UsedPercent)
		}
		window.CapacityPercent = 100
	} else if hasRemaining {
		window.Status = "仅提供剩余额度"
	} else {
		window.Unknown = true
		window.Status = "接口未返回额度数值"
	}
	if total <= 0 && !window.Unknown && hasPercentage {
		window.Status = "仅提供百分比"
	}
	return window
}

func glmUsageDetails(raw any) []map[string]any {
	switch values := raw.(type) {
	case []any:
		result := make([]map[string]any, 0, len(values))
		for _, value := range values {
			if item, ok := value.(map[string]any); ok {
				result = append(result, item)
			}
		}
		return result
	case map[string]any:
		result := make([]map[string]any, 0, len(values))
		for key, value := range values {
			if item, ok := value.(map[string]any); ok {
				if firstString(item, "modelCode", "model_code", "name") == "" {
					item["modelCode"] = key
				}
				result = append(result, item)
				continue
			}
			if count, ok := numberValue(value); ok {
				result = append(result, map[string]any{"modelCode": key, "usage": count})
			}
		}
		return result
	default:
		return nil
	}
}

func glmWindowLabel(limit glmLimitItem) string {
	if limit.Type == "TIME_LIMIT" {
		return "MCP 每月额度"
	}
	if limit.Type != "TOKENS_LIMIT" {
		return firstNonEmpty(limit.Type, "未知配额")
	}
	switch {
	case limit.Unit == 3 && limit.Number == 5:
		return "5 小时令牌额度"
	case limit.Unit == 6 && limit.Number == 1:
		return "每周令牌额度"
	case limit.Number > 0:
		return fmt.Sprintf("令牌额度（周期 %d，单位 %d）", limit.Number, limit.Unit)
	default:
		return "令牌额度"
	}
}

func glmToolLabel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "search-prime":
		return "联网搜索"
	case "web-reader":
		return "网页读取"
	case "zread":
		return "ZRead 调用"
	default:
		return value
	}
}

func (GLMZai) Fetch(authID, token, proxyURL string) balance.Result {
	return fetchGLMQuota(authID, token, proxyURL, "https://api.z.ai", balance.ProviderLabel[balance.ProviderGLMZAI])
}

func (GLMZhipu) Fetch(authID, token, proxyURL string) balance.Result {
	return fetchGLMQuota(authID, token, proxyURL, "https://open.bigmodel.cn", balance.ProviderLabel[balance.ProviderGLMZhipu])
}
