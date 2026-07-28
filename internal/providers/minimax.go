package providers

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Hamster-Prime/balance-query/internal/balance"
)

// MiniMaxAPI is intentionally explicit: MiniMax documents a model API key for
// inference, but does not publish an API-key-authenticated pay-as-you-go wallet
// endpoint. Sending the key to a guessed console URL is both inaccurate and
// unsafe.
type MiniMaxAPI struct{}

func (MiniMaxAPI) Fetch(authID, _, _ string) balance.Result {
	return errResult(authID, balance.ProviderLabel[balance.ProviderMiniMaxAPI],
		"官方未提供模型 API Key 的余额查询接口，请在 MiniMax 开放平台控制台查看按量余额")
}

type miniMaxQuotaResp struct {
	BaseResp struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp"`
	HasStatusCode   bool                 `json:"-"`
	ResponseError   string               `json:"-"`
	PlanTitle       string               `json:"current_subscribe_title"`
	PointsBalance   float64              `json:"points_balance"`
	HasPointBalance bool                 `json:"-"`
	ModelRemains    []miniMaxModelRemain `json:"model_remains"`
}

type miniMaxModelRemain struct {
	ModelName        string `json:"model_name"`
	HasCurrentFields bool   `json:"-"`
	HasWeeklyFields  bool   `json:"-"`

	StartTime   int64 `json:"start_time"`
	EndTime     int64 `json:"end_time"`
	RemainsTime int64 `json:"remains_time"` // milliseconds

	CurrentIntervalTotalCount   float64  `json:"current_interval_total_count"`
	CurrentIntervalUsageCount   float64  `json:"current_interval_usage_count"` // remaining, despite the name
	CurrentIntervalRemainingPct *float64 `json:"current_interval_remaining_percent"`
	IntervalBoostPermille       *float64 `json:"interval_boost_permille"`
	CurrentIntervalStatus       int      `json:"current_interval_status"`
	CurrentWeeklyTotalCount     float64  `json:"current_weekly_total_count"`
	CurrentWeeklyUsageCount     float64  `json:"current_weekly_usage_count"` // remaining, despite the name
	CurrentWeeklyRemainingPct   *float64 `json:"current_weekly_remaining_percent"`
	CurrentWeeklyStatus         int      `json:"current_weekly_status"`
	WeeklyStartTime             int64    `json:"weekly_start_time"`
	WeeklyEndTime               int64    `json:"weekly_end_time"`
	WeeklyRemainsTime           int64    `json:"weekly_remains_time"` // milliseconds
	WeeklyBoostPermille         *float64 `json:"weekly_boost_permille"`
}

func (response *miniMaxQuotaResp) UnmarshalJSON(data []byte) error {
	*response = miniMaxQuotaResp{}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	nested, _ := payload["data"].(map[string]any)
	base, _ := nested["base_resp"].(map[string]any)
	if _, exists := firstNumber(base, "status_code"); !exists {
		base, _ = payload["base_resp"].(map[string]any)
	}
	if code, ok := firstNumber(base, "status_code"); ok {
		response.BaseResp.StatusCode = int(code)
		response.HasStatusCode = true
	}
	response.BaseResp.StatusMsg = firstString(base, "status_msg")
	response.ResponseError = firstNonEmpty(firstString(nested, "message", "error"), firstString(payload, "message", "error"))
	response.PlanTitle = firstString(nested, "current_subscribe_title")
	if response.PlanTitle == "" {
		response.PlanTitle = firstString(payload, "current_subscribe_title")
	}
	if response.PlanTitle == "" {
		if subscription, ok := nested["current_subscribe"].(map[string]any); ok {
			response.PlanTitle = firstString(subscription, "current_subscribe_title")
		}
	}
	if response.PlanTitle == "" {
		if subscription, ok := payload["current_subscribe"].(map[string]any); ok {
			response.PlanTitle = firstString(subscription, "current_subscribe_title")
		}
	}
	response.PointsBalance, response.HasPointBalance = firstNumber(nested, "points_balance")
	if !response.HasPointBalance {
		response.PointsBalance, response.HasPointBalance = firstNumber(payload, "points_balance")
	}
	response.ModelRemains = nil
	rawModels, _ := nested["model_remains"].([]any)
	if rawModels == nil {
		rawModels, _ = payload["model_remains"].([]any)
	}
	for _, raw := range rawModels {
		model, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		response.ModelRemains = append(response.ModelRemains, miniMaxModelRemainFromMap(model))
	}
	return nil
}

func miniMaxModelRemainFromMap(values map[string]any) miniMaxModelRemain {
	number := func(keys ...string) float64 {
		value, _ := firstNumber(values, keys...)
		return value
	}
	integer := func(keys ...string) int64 {
		value, _ := int64Value(firstValue(values, keys...))
		return value
	}
	pointer := func(keys ...string) *float64 {
		value, ok := firstNumber(values, keys...)
		if !ok {
			return nil
		}
		return &value
	}
	return miniMaxModelRemain{
		ModelName:                   firstString(values, "model_name"),
		HasCurrentFields:            hasAnyMapKey(values, "start_time", "end_time", "remains_time", "current_interval_total_count", "current_interval_usage_count", "current_interval_remaining_percent", "current_interval_status", "interval_boost_permille", "interval_boost_permill"),
		HasWeeklyFields:             hasAnyMapKey(values, "current_weekly_total_count", "current_weekly_usage_count", "current_weekly_remaining_percent", "current_weekly_status", "weekly_start_time", "weekly_end_time", "weekly_remains_time", "weekly_boost_permille", "weekly_boost_permill"),
		StartTime:                   integer("start_time"),
		EndTime:                     integer("end_time"),
		RemainsTime:                 integer("remains_time"),
		CurrentIntervalTotalCount:   number("current_interval_total_count"),
		CurrentIntervalUsageCount:   number("current_interval_usage_count"),
		CurrentIntervalRemainingPct: pointer("current_interval_remaining_percent"),
		IntervalBoostPermille:       pointer("interval_boost_permille", "interval_boost_permill"),
		CurrentIntervalStatus:       int(number("current_interval_status")),
		CurrentWeeklyTotalCount:     number("current_weekly_total_count"),
		CurrentWeeklyUsageCount:     number("current_weekly_usage_count"),
		CurrentWeeklyRemainingPct:   pointer("current_weekly_remaining_percent"),
		CurrentWeeklyStatus:         int(number("current_weekly_status")),
		WeeklyStartTime:             integer("weekly_start_time"),
		WeeklyEndTime:               integer("weekly_end_time"),
		WeeklyRemainsTime:           integer("weekly_remains_time"),
		WeeklyBoostPermille:         pointer("weekly_boost_permille", "weekly_boost_permill"),
	}
}

func hasAnyMapKey(values map[string]any, keys ...string) bool {
	for _, key := range keys {
		if _, exists := values[key]; exists {
			return true
		}
	}
	return false
}

func fetchMiniMaxCoding(authID, token, proxyURL, apiBase, label string) balance.Result {
	endpoint := apiBase + "/v1/token_plan/remains"
	if strings.TrimSpace(token) == "" {
		return errResult(authID, label, "接口密钥为空")
	}

	// MiniMax's official CLI probes both credential styles. A key can receive
	// HTTP 200 with a non-zero business status under the wrong style, so only a
	// fully successful business response stops the probe.
	authHeaders := []map[string]string{
		{"Authorization": "Bearer " + token},
		{"x-api-key": token},
	}
	lastMessage := "MiniMax 配额查询失败"
	for _, headers := range authHeaders {
		var resp miniMaxQuotaResp
		if err := getJSONWithHeaders(endpoint, proxyURL, headers, &resp); err != nil {
			lastMessage = err.Error()
			continue
		}
		if resp.BaseResp.StatusCode != 0 {
			lastMessage = strings.TrimSpace(resp.BaseResp.StatusMsg)
			if lastMessage == "" {
				lastMessage = fmt.Sprintf("MiniMax 配额接口返回业务错误 %d", resp.BaseResp.StatusCode)
			}
			continue
		}
		if !resp.HasStatusCode && len(resp.ModelRemains) == 0 {
			lastMessage = firstNonEmpty(strings.TrimSpace(resp.ResponseError), "MiniMax 配额接口未返回有效业务状态")
			continue
		}
		if len(resp.ModelRemains) == 0 {
			result := parseMiniMaxQuota(authID, label, resp)
			result.QuotaWindows = []balance.QuotaWindow{{
				Group:            "Token Plan",
				Label:            "当前配额",
				Unavailable:      true,
				Status:           "暂无可用配额数据",
				AggregationScope: "key",
			}}
			return result
		}
		return parseMiniMaxQuota(authID, label, resp)
	}
	return errResult(authID, label, lastMessage)
}

func parseMiniMaxQuota(authID, label string, resp miniMaxQuotaResp) balance.Result {
	r := balance.Result{
		Provider:  label,
		AuthID:    authID,
		Plan:      firstNonEmpty(resp.PlanTitle, "Token Plan"),
		FetchedAt: time.Now(),
		Extra:     map[string]string{},
	}
	primarySet := false
	for _, model := range resp.ModelRemains {
		group := miniMaxModelLabel(model.ModelName)
		currentFieldsPresent := model.HasCurrentFields || model.CurrentIntervalTotalCount != 0 ||
			model.CurrentIntervalUsageCount != 0 || model.CurrentIntervalRemainingPct != nil || model.CurrentIntervalStatus != 0
		weeklyFieldsPresent := model.HasWeeklyFields || model.CurrentWeeklyTotalCount != 0 ||
			model.CurrentWeeklyUsageCount != 0 || model.CurrentWeeklyRemainingPct != nil || model.CurrentWeeklyStatus != 0
		unavailable := model.CurrentIntervalTotalCount == 0 &&
			model.CurrentWeeklyTotalCount == 0 &&
			model.CurrentIntervalStatus == 3 && model.CurrentWeeklyStatus == 3

		intervalSeconds := (model.EndTime - model.StartTime) / 1000
		intervalBoost := 1.0
		if model.IntervalBoostPermille != nil {
			intervalBoost = maxFloat(*model.IntervalBoostPermille, 0) / 1000
		}
		current := miniMaxWindow(group, durationWindowLabel(intervalSeconds),
			model.CurrentIntervalTotalCount, model.CurrentIntervalUsageCount,
			model.CurrentIntervalRemainingPct, model.CurrentIntervalStatus,
			model.EndTime, model.RemainsTime, unavailable, intervalBoost, false, currentFieldsPresent)
		weeklyBoost := 1.0
		if model.WeeklyBoostPermille != nil {
			weeklyBoost = maxFloat(*model.WeeklyBoostPermille, 0) / 1000
		}
		weekly := miniMaxWindow(group, "每周配额",
			model.CurrentWeeklyTotalCount, model.CurrentWeeklyUsageCount,
			model.CurrentWeeklyRemainingPct, model.CurrentWeeklyStatus,
			model.WeeklyEndTime, model.WeeklyRemainsTime, unavailable, weeklyBoost, true, weeklyFieldsPresent)

		r.QuotaWindows = append(r.QuotaWindows, current, weekly)
		if !primarySet && !current.Unavailable && (current.Total > 0 || current.RemainingPercent > 0) {
			applyPrimaryWindow(&r, current)
			primarySet = true
		}
		if model.WeeklyBoostPermille != nil && *model.WeeklyBoostPermille != 1000 {
			r.Extra[group+"周额度倍率"] = fmt.Sprintf("%.1f%%", *model.WeeklyBoostPermille/10)
		}
		if model.IntervalBoostPermille != nil && *model.IntervalBoostPermille != 1000 {
			r.Extra[group+"短周期额度倍率"] = fmt.Sprintf("%.1f%%", *model.IntervalBoostPermille/10)
		}
	}
	if resp.HasPointBalance {
		r.Extra["积分余额"] = formatQuotaNumber(resp.PointsBalance)
	}
	for _, model := range resp.ModelRemains {
		if model.WeeklyStartTime > 0 || model.WeeklyEndTime > 0 {
			r.Extra["周周期"] = fmt.Sprintf("%s 至 %s", formatUnixTimestamp(model.WeeklyStartTime), formatUnixTimestamp(model.WeeklyEndTime))
			break
		}
	}
	if len(r.Extra) == 0 {
		r.Extra = nil
	}
	return r
}

func miniMaxWindow(group, label string, total, remaining float64, remainingPct *float64, status int, resetAt, resetInMS int64, unavailable bool, boost float64, allowUnlimited, fieldsPresent bool) balance.QuotaWindow {
	windowKind := "interval"
	if allowUnlimited {
		windowKind = "weekly"
	}
	capacityPercent := 100 * boost
	if capacityPercent <= 0 {
		capacityPercent = 100
	}
	window := balance.QuotaWindow{
		Group:            group,
		Label:            label,
		Total:            total,
		Remaining:        remaining,
		Used:             maxFloat(total-remaining, 0),
		Unit:             "次",
		ResetAt:          formatUnixTimestamp(resetAt),
		ResetInSeconds:   maxInt64(resetInMS/1000, 0),
		Unavailable:      unavailable,
		AggregationScope: "key",
		AggregationKey:   "minimax:" + strings.ToLower(strings.TrimSpace(group)) + ":" + windowKind,
		CapacityPercent:  capacityPercent,
	}
	if unavailable {
		window.Status = "不在当前套餐中"
		return window
	}
	if !fieldsPresent {
		window.Unknown = true
		window.Status = "接口未返回此配额"
		return window
	}
	if allowUnlimited && status == 3 {
		window.Unlimited = true
		window.Status = "无限制"
	} else if status == 2 {
		window.Status = "已用尽"
	} else {
		window.Status = "正常"
	}
	if !window.Unlimited && remainingPct == nil && total <= 0 && remaining <= 0 && status != 2 {
		window.Unknown = true
		window.Status = "接口未返回额度数值"
		return window
	}
	if remainingPct != nil {
		window.RemainingPercent = clampDisplayPercent(*remainingPct * boost)
	} else if total > 0 {
		window.RemainingPercent = clampDisplayPercent(remaining / total * 100 * boost)
	}
	window.UsedPercent = maxFloat(capacityPercent-window.RemainingPercent, 0)
	return window
}

func clampDisplayPercent(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 200 {
		return 200
	}
	return value
}

func miniMaxModelLabel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "general":
		return "通用模型"
	case "video":
		return "视频模型"
	default:
		if strings.TrimSpace(value) == "" {
			return "默认资源"
		}
		return value
	}
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

// MiniMaxCodingCN queries MiniMax Token Plan quota in mainland China.
type MiniMaxCodingCN struct{}

func (MiniMaxCodingCN) Fetch(authID, token, proxyURL string) balance.Result {
	return fetchMiniMaxCoding(authID, token, proxyURL, "https://api.minimaxi.com",
		balance.ProviderLabel[balance.ProviderMiniMaxCodingCN])
}

// MiniMaxCodingGlobal queries MiniMax Token Plan quota outside mainland China.
type MiniMaxCodingGlobal struct{}

func (MiniMaxCodingGlobal) Fetch(authID, token, proxyURL string) balance.Result {
	return fetchMiniMaxCoding(authID, token, proxyURL, "https://api.minimax.io",
		balance.ProviderLabel[balance.ProviderMiniMaxCodingGlobal])
}
