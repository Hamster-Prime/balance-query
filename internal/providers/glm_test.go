package providers

import (
	"testing"
	"time"

	"github.com/Hamster-Prime/balance-query/internal/balance"
)

func TestGLMUsageRangeMatchesOfficialTwentyFourHourWindow(t *testing.T) {
	location := time.FixedZone("CST", 8*60*60)
	start, end := glmUsageRange(time.Date(2026, 7, 27, 16, 23, 10, 0, location))
	if start != "2026-07-26 16:00:00" || end != "2026-07-27 16:59:59" {
		t.Fatalf("range = %q..%q", start, end)
	}
}

func TestAppendGLMModelAndToolDetails(t *testing.T) {
	result := balance.Result{Extra: map[string]string{}}
	modelData := map[string]any{
		"totalUsage": map[string]any{
			"totalModelCallCount": float64(1072),
			"totalTokensUsage":    float64(84739459),
			"modelSummaryList": []any{
				map[string]any{"modelName": "GLM-5.1", "totalTokens": float64(70000000)},
			},
		},
	}
	appendGLMModelDetails(&result, modelData)
	if result.Extra["24 小时模型调用"] != "1072 次" || result.Extra["24 小时令牌用量"] != "84739459" {
		t.Fatalf("model totals = %#v", result.Extra)
	}
	if result.Extra["24 小时 GLM-5.1"] != "70000000 令牌" {
		t.Fatalf("model breakdown = %#v", result.Extra)
	}

	toolData := map[string]any{
		"totalUsage": map[string]any{
			"totalNetworkSearchCount": float64(3),
			"toolSummaryList": []any{
				map[string]any{"toolNameI18n": "网页读取", "totalUsageCount": float64(9)},
			},
		},
	}
	appendGLMToolDetails(&result, toolData)
	if result.Extra["24 小时联网搜索"] != "3 次" || result.Extra["24 小时 网页读取"] != "9 次" {
		t.Fatalf("tool details = %#v", result.Extra)
	}
}

func TestGLMQuotaUsesUsageAsTotalAndExplicitRemaining(t *testing.T) {
	remaining := 30_000_000.0
	window := glmQuotaWindow(glmLimitItem{
		Type:         "TOKENS_LIMIT",
		Unit:         3,
		Number:       5,
		Usage:        40_000_000,
		CurrentValue: testFloat64(10_000_000),
		Remaining:    &remaining,
		Percentage:   testFloat64(25),
	})
	if window.Total != 40_000_000 || window.Used != 10_000_000 || window.Remaining != 30_000_000 {
		t.Fatalf("quota window = %#v", window)
	}
	if window.Label != "5 小时令牌额度" {
		t.Fatalf("label = %q", window.Label)
	}
	if window.AggregationScope != "key" {
		t.Fatalf("aggregation scope = %q, want key", window.AggregationScope)
	}
}

func TestGLMMissingWeeklyWindowMeansUnlimitedWhenFiveHourWindowExists(t *testing.T) {
	result := balance.Result{}
	appendGLMWeeklyUnlimitedIfMissing(&result, []glmLimitItem{
		{Type: "TOKENS_LIMIT", Unit: 3, Number: 5, Percentage: testFloat64(20)},
	})
	if len(result.QuotaWindows) != 1 {
		t.Fatalf("GLM synthesized weekly windows = %#v", result.QuotaWindows)
	}
	window := result.QuotaWindows[0]
	if !window.Unlimited || window.Label != "每周令牌额度" || window.Status != "不限量" || window.AggregationKey != "glm:TOKENS_LIMIT:6:1" {
		t.Fatalf("GLM synthesized weekly window = %#v", window)
	}
}

func TestGLMDoesNotSynthesizeWeeklyUnlimitedWhenWeeklyRowExists(t *testing.T) {
	result := balance.Result{}
	appendGLMWeeklyUnlimitedIfMissing(&result, []glmLimitItem{
		{Type: "TOKENS_LIMIT", Unit: 3, Number: 5},
		{Type: "TOKENS_LIMIT", Unit: 6, Number: 1},
	})
	if len(result.QuotaWindows) != 0 {
		t.Fatalf("GLM existing weekly window was duplicated: %#v", result.QuotaWindows)
	}
}

func TestGLMDoesNotInferUnlimitedFromIncompleteLimits(t *testing.T) {
	result := balance.Result{}
	appendGLMWeeklyUnlimitedIfMissing(&result, []glmLimitItem{{Type: "TIME_LIMIT"}})
	if len(result.QuotaWindows) != 0 {
		t.Fatalf("GLM incomplete limits inferred weekly unlimited: %#v", result.QuotaWindows)
	}
}

func TestGLMDoesNotInferUnlimitedFromIncompleteFiveHourRow(t *testing.T) {
	result := balance.Result{}
	appendGLMWeeklyUnlimitedIfMissing(&result, []glmLimitItem{{Type: "TOKENS_LIMIT", Unit: 3, Number: 5}})
	if len(result.QuotaWindows) != 0 {
		t.Fatalf("GLM incomplete 5-hour row inferred weekly unlimited: %#v", result.QuotaWindows)
	}
}
