package providers

import (
	"encoding/json"
	"testing"

	"github.com/Hamster-Prime/balance-query/internal/balance"
)

func TestNumberValueSupportsJSONNumber(t *testing.T) {
	got, ok := numberValue(json.Number("12.75"))
	if !ok || got != 12.75 {
		t.Fatalf("numberValue(json.Number) = (%v, %v)", got, ok)
	}
}

func TestParseKimiUsageTopLevelWindowsAndBooster(t *testing.T) {
	result := parseKimiUsage("kimi", kimiUsageResp{
		Usage: map[string]any{
			"name": "Weekly limit", "used": float64(40), "limit": float64(1000),
			"resetAt": "2026-08-01T00:00:00Z",
		},
		Limits: []map[string]any{{
			"detail": map[string]any{"remaining": float64(80), "limit": float64(100)},
			"window": map[string]any{"duration": float64(300), "timeUnit": "MINUTE"},
		}},
		BoosterWallet: map[string]any{
			"balance": map[string]any{
				"type": "BOOSTER", "amount": "20000000000", "amountLeft": "10000000000",
			},
			"monthlyChargeLimitEnabled": true,
			"monthlyChargeLimit":        map[string]any{"currency": "USD", "priceInCents": "20000"},
			"monthlyUsed":               map[string]any{"currency": "USD", "priceInCents": "5000"},
		},
	})

	if result.Error != "" {
		t.Fatalf("parseKimiUsage() error = %q", result.Error)
	}
	if len(result.QuotaWindows) != 3 {
		t.Fatalf("quota windows = %d, want 3: %#v", len(result.QuotaWindows), result.QuotaWindows)
	}
	if got := result.QuotaWindows[0]; got.Label != "每周配额" || got.RemainingPercent != 96 || got.UsedPercent != 4 || got.Unit != "" {
		t.Fatalf("weekly window = %#v", got)
	}
	if got := result.QuotaWindows[1]; got.Label != "5 小时配额" || got.UsedPercent != 20 || got.RemainingPercent != 80 || got.Unit != "" {
		t.Fatalf("short window = %#v", got)
	}
	if got := result.QuotaDisplay; got != "每周配额：剩余 96%" {
		t.Fatalf("quota display = %q", got)
	}
	if got := result.QuotaWindows[2]; got.Label != "每月付费上限" || got.Total != 200 || got.Used != 50 {
		t.Fatalf("booster monthly window = %#v", got)
	}
	for _, window := range result.QuotaWindows {
		if window.AggregationScope != "key" {
			t.Fatalf("Kimi window scope = %q, want key: %#v", window.AggregationScope, window)
		}
	}
	if got := result.Extra["加量包余额"]; got != "100.00 / 200.00 USD" {
		t.Fatalf("booster balance = %q", got)
	}
}

func TestKimiQuotaWindowClampsDerivedValuesAndReadsWindowReset(t *testing.T) {
	window, ok := kimiQuotaWindow(map[string]any{
		"limit":     float64(100),
		"remaining": float64(150),
		"window":    "3600",
	}, "短周期配额")
	if !ok {
		t.Fatal("kimiQuotaWindow() did not recognize quota")
	}
	if window.Used != 0 || window.Remaining != 0 || window.UsedPercent != 0 || window.RemainingPercent != 100 {
		t.Fatalf("clamped quota = %#v", window)
	}
	if window.ResetInSeconds != 3600 {
		t.Fatalf("reset seconds = %d, want 3600", window.ResetInSeconds)
	}
}

func TestKimiQuotaIsPercentageNotRequestCount(t *testing.T) {
	result := parseKimiUsage("kimi", kimiUsageResp{
		Usage: map[string]any{
			"name": "Weekly limit", "used": float64(250), "limit": float64(1000),
		},
		Limits: []map[string]any{{
			"detail": map[string]any{"used": float64(3), "limit": float64(10)},
			"window": map[string]any{"duration": float64(300), "timeUnit": "TIME_UNIT_MINUTE"},
		}},
	})

	if len(result.QuotaWindows) != 2 {
		t.Fatalf("quota windows = %#v", result.QuotaWindows)
	}
	for _, window := range result.QuotaWindows {
		if window.Unit != "" || window.Total != 0 || window.Used != 0 || window.Remaining != 0 {
			t.Fatalf("Kimi quota leaked raw ratio as a count: %#v", window)
		}
	}
	if got := result.QuotaWindows[0]; got.UsedPercent != 25 || got.RemainingPercent != 75 {
		t.Fatalf("weekly percentage = %#v", got)
	}
	if got := result.QuotaWindows[1]; got.Label != "5 小时配额" || got.UsedPercent != 30 || got.RemainingPercent != 70 {
		t.Fatalf("rolling percentage = %#v", got)
	}
}

func TestKimiBoosterUsesOfficialWholeCentRounding(t *testing.T) {
	result := balance.Result{Extra: map[string]string{}}
	parseKimiBoosterWallet(&result, map[string]any{
		"balance": map[string]any{
			"type": "BOOSTER", "amount": float64(1_500_000), "amountLeft": float64(1),
		},
	})
	if got := result.Extra["加量包余额"]; got != "0.01 / 0.02 USD" {
		t.Fatalf("rounded booster balance = %q", got)
	}
}

func TestParseSub2APIQuotaLimitedDetails(t *testing.T) {
	result := parseSub2APIUsage("sub", map[string]any{
		"mode":    "quota_limited",
		"isValid": true,
		"quota": map[string]any{
			"limit": float64(100), "used": float64(25), "remaining": float64(75), "unit": "USD",
		},
		"rate_limits": []any{
			map[string]any{"window": "5h", "limit": float64(10), "used": float64(3), "remaining": float64(7), "reset_at": "2026-07-27T12:00:00Z"},
			map[string]any{"window": "7d", "limit": float64(50), "used": float64(20), "remaining": float64(30)},
		},
		"usage": map[string]any{
			"today": map[string]any{"requests": float64(9), "total_tokens": float64(1234), "actual_cost": float64(1.25)},
			"total": map[string]any{"requests": float64(99), "total_tokens": float64(9999)},
			"rpm":   float64(2), "tpm": float64(3500),
		},
	})

	if result.Error != "" {
		t.Fatalf("parseSub2APIUsage() error = %q", result.Error)
	}
	if len(result.QuotaWindows) != 3 {
		t.Fatalf("quota windows = %d, want 3", len(result.QuotaWindows))
	}
	if result.QuotaWindows[1].Label != "5 小时额度" || result.QuotaWindows[2].Label != "7 天额度" {
		t.Fatalf("rate limit labels = %#v", result.QuotaWindows)
	}
	for _, key := range []string{"今日请求", "今日令牌", "累计请求", "累计令牌", "当前 RPM", "当前 TPM"} {
		if result.Extra[key] == "" {
			t.Fatalf("missing usage detail %q in %#v", key, result.Extra)
		}
	}
}

func TestSub2APIMonetarySummaryPreservesDecimals(t *testing.T) {
	result := parseSub2APIUsage("sub", map[string]any{
		"mode": "quota_limited",
		"quota": map[string]any{
			"limit": float64(10.5), "used": float64(3.25), "remaining": float64(7.25), "unit": "USD",
		},
	})
	if got := result.QuotaDisplay; got != "总额度：剩余 7.25 / 10.5 USD" {
		t.Fatalf("monetary quota display = %q", got)
	}
}

func TestSub2APINonUSDBalanceRemainsStructuredAlongsideQuota(t *testing.T) {
	result := parseSub2APIUsage("sub", map[string]any{
		"mode":    "quota_limited",
		"unit":    "CNY",
		"balance": float64(12.5),
		"quota": map[string]any{
			"limit": float64(100), "used": float64(20), "remaining": float64(80), "unit": "CNY",
		},
	})
	if !result.HasBalanceAmount || result.BalanceAmount != 12.5 || result.BalanceCurrency != "CNY" {
		t.Fatalf("structured non-USD balance = %#v", result)
	}
	if result.HasBalance || result.BalanceUSD != 0 {
		t.Fatalf("non-USD balance leaked into USD fields: %#v", result)
	}
	if result.BalanceScope != "account" || len(result.QuotaWindows) != 1 {
		t.Fatalf("balance/quota metadata = %#v", result)
	}
}

func TestParseSub2APISubscriptionWindows(t *testing.T) {
	result := parseSub2APIUsage("sub", map[string]any{
		"mode":      "unrestricted",
		"isValid":   true,
		"planName":  "专业版",
		"unit":      "USD",
		"remaining": float64(5),
		"subscription": map[string]any{
			"daily_usage_usd": float64(1), "daily_limit_usd": float64(10),
			"weekly_usage_usd": float64(4), "weekly_limit_usd": float64(20),
			"monthly_usage_usd": float64(8), "monthly_limit_usd": float64(50),
			"weekly_window_start": "2026-07-20T00:00:00Z",
		},
	})
	if len(result.QuotaWindows) != 3 {
		t.Fatalf("subscription windows = %d, want 3: %#v", len(result.QuotaWindows), result.QuotaWindows)
	}
	for _, window := range result.QuotaWindows {
		if window.AggregationScope != "account" {
			t.Fatalf("subscription scope = %q, want account: %#v", window.AggregationScope, window)
		}
	}
	if result.QuotaWindows[2].Label != "每月额度" || result.QuotaWindows[2].Remaining != 42 {
		t.Fatalf("monthly window = %#v", result.QuotaWindows[2])
	}
	if got := result.QuotaWindows[1].ResetAt; got != "2026-07-27T00:00:00Z" {
		t.Fatalf("weekly reset = %q, want end of seven-day window", got)
	}
}

func TestParseSub2APIStatusIsLocalized(t *testing.T) {
	result := parseSub2APIUsage("sub", map[string]any{
		"mode": "quota_limited", "status": "quota_exhausted",
		"quota": map[string]any{"limit": float64(10), "used": float64(10)},
	})
	if got := result.Extra["状态详情"]; got != "额度已用尽" {
		t.Fatalf("localized status = %q", got)
	}
}

func TestSub2APIHistoryIncludesRecentDaysAndTopModels(t *testing.T) {
	result := balance.Result{Extra: map[string]string{}}
	appendSub2APIHistoryDetails(&result, map[string]any{
		"daily_usage": []any{
			map[string]any{"date": "2026-07-26", "requests": float64(2), "total_tokens": float64(100), "actual_cost": float64(0.2)},
			map[string]any{"date": "2026-07-27", "requests": float64(3), "total_tokens": float64(200), "actual_cost": float64(0.3)},
		},
		"model_stats": []any{
			map[string]any{"model": "model-small", "requests": float64(9), "total_tokens": float64(900), "actual_cost": float64(0.1)},
			map[string]any{"model": "model-large", "requests": float64(4), "total_tokens": float64(800), "actual_cost": float64(1.2)},
		},
	})
	if result.Extra["近 30 天请求"] != "5 次" || result.Extra["近 30 天令牌"] != "300" {
		t.Fatalf("history totals = %#v", result.Extra)
	}
	if result.Extra["2026-07-27 用量"] == "" || result.Extra["模型 model-large"] == "" {
		t.Fatalf("history details = %#v", result.Extra)
	}
}

func TestMiniMaxWindowUsesUsageCountAsRemaining(t *testing.T) {
	remainingPct := 60.0
	window := miniMaxWindow("通用模型", "5 小时配额", 100, 60, &remainingPct, 1, 1_800_000_000_000, 3_600_000, false, 1, false)
	if window.Remaining != 60 || window.Used != 40 || window.RemainingPercent != 60 {
		t.Fatalf("MiniMax window = %#v", window)
	}
	if window.ResetInSeconds != 3600 {
		t.Fatalf("reset seconds = %d, want 3600", window.ResetInSeconds)
	}
}

func TestMiniMaxUnlimitedAndUnavailableAreDistinct(t *testing.T) {
	unlimited := miniMaxWindow("通用模型", "每周配额", 0, 0, nil, 3, 0, 0, false, 1, true)
	if !unlimited.Unlimited || unlimited.Unavailable {
		t.Fatalf("unlimited window = %#v", unlimited)
	}
	unavailable := miniMaxWindow("视频模型", "每周配额", 0, 0, nil, 3, 0, 0, true, 1, true)
	if unavailable.Unlimited || !unavailable.Unavailable || unavailable.Status != "不在当前套餐中" {
		t.Fatalf("unavailable window = %#v", unavailable)
	}
}

func TestMiniMaxCurrentWindowDoesNotTreatStatusThreeAsWeeklyUnlimited(t *testing.T) {
	current := miniMaxWindow("通用模型", "5 小时配额", 100, 50, nil, 3, 0, 0, false, 1, false)
	if current.Unlimited || current.Status != "正常" {
		t.Fatalf("current interval = %#v", current)
	}
}

func TestParseMiniMaxQuotaKeepsDetailedSummaryAndSkipsUnavailableFirstModel(t *testing.T) {
	resp := miniMaxQuotaResp{ModelRemains: []miniMaxModelRemain{
		{
			ModelName: "video", CurrentIntervalStatus: 3, CurrentWeeklyStatus: 3,
		},
		{
			ModelName: "general", StartTime: 1_800_000_000_000, EndTime: 1_800_018_000_000,
			CurrentIntervalTotalCount: 100, CurrentIntervalUsageCount: 75,
			CurrentWeeklyTotalCount: 1000, CurrentWeeklyUsageCount: 900,
		},
	}}
	result := parseMiniMaxQuota("mini", "MiniMax Token Plan（测试）", resp)
	if result.Error != "" {
		t.Fatalf("parseMiniMaxQuota() error = %q", result.Error)
	}
	if result.TokensTotal != 100 || result.TokensRemaining != 75 || result.TokensUsed != 25 {
		t.Fatalf("primary quota = %#v", result)
	}
	if result.QuotaDisplay != "5 小时配额：剩余 75 / 100 次" {
		t.Fatalf("quota display = %q", result.QuotaDisplay)
	}
}

func TestMiniMaxQuotaResponseAcceptsNestedStringFieldsAndBoostAliases(t *testing.T) {
	var response miniMaxQuotaResp
	err := json.Unmarshal([]byte(`{
		"base_resp":{"status_code":"0"},
		"data":{"current_subscribe_title":"Token Plan Plus","points_balance":"14000","model_remains":[{
			"model_name":"general",
			"start_time":"1800000000000",
			"end_time":"1800018000000",
			"remains_time":"3600000",
			"current_interval_total_count":"100",
			"current_interval_usage_count":"80",
			"current_interval_remaining_percent":"80",
			"current_interval_status":"1",
			"interval_boost_permille":"1250",
			"current_weekly_total_count":"1000",
			"current_weekly_usage_count":"900",
			"current_weekly_remaining_percent":"90",
			"current_weekly_status":"1",
			"weekly_boost_permill":"1500"
		}]}
	}`), &response)
	if err != nil {
		t.Fatalf("unmarshal MiniMax nested response: %v", err)
	}
	if len(response.ModelRemains) != 1 {
		t.Fatalf("nested model remains = %#v", response.ModelRemains)
	}
	if response.PlanTitle != "Token Plan Plus" || !response.HasPointBalance || response.PointsBalance != 14000 {
		t.Fatalf("nested plan metadata = %#v", response)
	}
	model := response.ModelRemains[0]
	if model.CurrentIntervalTotalCount != 100 || model.CurrentIntervalUsageCount != 80 || model.CurrentIntervalStatus != 1 {
		t.Fatalf("string quota fields = %#v", model)
	}
	if model.IntervalBoostPermille == nil || *model.IntervalBoostPermille != 1250 || model.WeeklyBoostPermille == nil || *model.WeeklyBoostPermille != 1500 {
		t.Fatalf("boost aliases = %#v", model)
	}
	result := parseMiniMaxQuota("mini", "MiniMax Token Plan（测试）", response)
	if result.Plan != "Token Plan Plus" || result.Extra["积分余额"] != "14000" {
		t.Fatalf("parsed plan metadata = %#v", result)
	}
	if got := result.QuotaWindows[0].RemainingPercent; got != 100 {
		t.Fatalf("interval boosted remaining = %v, want 100", got)
	}
	if got := result.QuotaWindows[1].RemainingPercent; got != 135 {
		t.Fatalf("weekly boosted remaining = %v, want 135", got)
	}
}

func TestMiniMaxQuotaResponseFallsBackAcrossMixedEnvelope(t *testing.T) {
	var response miniMaxQuotaResp
	err := json.Unmarshal([]byte(`{
		"base_resp":{"status_code":"0"},
		"current_subscribe_title":"Token Plan Pro",
		"points_balance":"21",
		"model_remains":[{"model_name":"general","current_interval_total_count":"10","current_interval_usage_count":"9"}],
		"data":{"current_subscribe_title":"Token Plan Plus"}
	}`), &response)
	if err != nil {
		t.Fatalf("unmarshal mixed MiniMax response: %v", err)
	}
	if !response.HasStatusCode || response.PlanTitle != "Token Plan Plus" || response.PointsBalance != 21 || len(response.ModelRemains) != 1 {
		t.Fatalf("mixed MiniMax envelope = %#v", response)
	}
}

func TestGLMWindowUsesPeriodMetadataNotNumberAsQuota(t *testing.T) {
	window := glmQuotaWindow(glmLimitItem{
		Type: "TOKENS_LIMIT", Unit: 3, Number: 5, Percentage: 25, Total: 40_000_000,
	})
	if window.Label != "5 小时令牌额度" || window.Total != 40_000_000 || window.Used != 10_000_000 {
		t.Fatalf("GLM window = %#v", window)
	}
	if window.UsedPercent != 25 || window.RemainingPercent != 75 {
		t.Fatalf("GLM percentages = %#v", window)
	}
}

func TestQuotaWindowJSONFieldNameIsStable(t *testing.T) {
	_ = balance.QuotaWindow{Label: "每周配额"}
}
