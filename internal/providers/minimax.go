package providers

import (
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
	ModelRemains []miniMaxModelRemain `json:"model_remains"`
}

type miniMaxModelRemain struct {
	ModelName string `json:"model_name"`

	StartTime   int64 `json:"start_time"`
	EndTime     int64 `json:"end_time"`
	RemainsTime int64 `json:"remains_time"` // milliseconds

	CurrentIntervalTotalCount   float64  `json:"current_interval_total_count"`
	CurrentIntervalUsageCount   float64  `json:"current_interval_usage_count"` // remaining, despite the name
	CurrentIntervalRemainingPct *float64 `json:"current_interval_remaining_percent"`
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

func fetchMiniMaxCoding(authID, token, proxyURL, apiBase, label string) balance.Result {
	endpoint := apiBase + "/v1/token_plan/remains"
	var resp miniMaxQuotaResp
	err := getJSON(endpoint, token, proxyURL, &resp)
	if err != nil {
		// The official CLI probes both supported credential styles because key
		// types differ between MiniMax products and regions.
		resp = miniMaxQuotaResp{}
		if retryErr := getJSONWithHeaders(endpoint, proxyURL, map[string]string{"x-api-key": token}, &resp); retryErr != nil {
			return errResult(authID, label, err.Error())
		}
	}
	if resp.BaseResp.StatusCode != 0 {
		message := strings.TrimSpace(resp.BaseResp.StatusMsg)
		if message == "" {
			message = fmt.Sprintf("MiniMax 配额接口返回业务错误 %d", resp.BaseResp.StatusCode)
		}
		return errResult(authID, label, message)
	}
	if len(resp.ModelRemains) == 0 {
		return errResult(authID, label, "官方接口未返回 Token Plan 配额数据")
	}
	return parseMiniMaxQuota(authID, label, resp)
}

func parseMiniMaxQuota(authID, label string, resp miniMaxQuotaResp) balance.Result {
	r := balance.Result{
		Provider:  label,
		AuthID:    authID,
		Plan:      "Token Plan",
		FetchedAt: time.Now(),
		Extra:     map[string]string{},
	}
	primarySet := false
	for _, model := range resp.ModelRemains {
		group := miniMaxModelLabel(model.ModelName)
		unavailable := model.CurrentIntervalTotalCount == 0 &&
			model.CurrentWeeklyTotalCount == 0 &&
			model.CurrentIntervalStatus == 3 && model.CurrentWeeklyStatus == 3

		intervalSeconds := (model.EndTime - model.StartTime) / 1000
		current := miniMaxWindow(group, durationWindowLabel(intervalSeconds),
			model.CurrentIntervalTotalCount, model.CurrentIntervalUsageCount,
			model.CurrentIntervalRemainingPct, model.CurrentIntervalStatus,
			model.EndTime, model.RemainsTime, unavailable, 1, false)
		weeklyBoost := 1.0
		if model.WeeklyBoostPermille != nil {
			weeklyBoost = maxFloat(*model.WeeklyBoostPermille, 0) / 1000
		}
		weekly := miniMaxWindow(group, "每周配额",
			model.CurrentWeeklyTotalCount, model.CurrentWeeklyUsageCount,
			model.CurrentWeeklyRemainingPct, model.CurrentWeeklyStatus,
			model.WeeklyEndTime, model.WeeklyRemainsTime, unavailable, weeklyBoost, true)

		r.QuotaWindows = append(r.QuotaWindows, current, weekly)
		if !primarySet && !current.Unavailable && (current.Total > 0 || current.RemainingPercent > 0) {
			applyPrimaryWindow(&r, current)
			primarySet = true
		}
		if model.WeeklyBoostPermille != nil && *model.WeeklyBoostPermille != 1000 {
			r.Extra[group+"周额度倍率"] = fmt.Sprintf("%.1f%%", *model.WeeklyBoostPermille/10)
		}
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

func miniMaxWindow(group, label string, total, remaining float64, remainingPct *float64, status int, resetAt, resetInMS int64, unavailable bool, boost float64, allowUnlimited bool) balance.QuotaWindow {
	window := balance.QuotaWindow{
		Group:          group,
		Label:          label,
		Total:          total,
		Remaining:      remaining,
		Used:           maxFloat(total-remaining, 0),
		Unit:           "次",
		ResetAt:        formatUnixTimestamp(resetAt),
		ResetInSeconds: maxInt64(resetInMS/1000, 0),
		Unavailable:    unavailable,
	}
	if unavailable {
		window.Status = "不在当前套餐中"
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
	if remainingPct != nil {
		window.RemainingPercent = clampDisplayPercent(*remainingPct * boost)
	} else if total > 0 {
		window.RemainingPercent = clampDisplayPercent(remaining / total * 100 * boost)
	}
	window.UsedPercent = clampPercent(100 - window.RemainingPercent)
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
