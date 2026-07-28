package ui

import (
	"encoding/json"
	"math"
	"os/exec"
	"strings"
	"testing"
)

func runDashboardAggregation(t *testing.T, results string) []map[string]any {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	page := string(RenderDashboard(300))
	start := strings.Index(page, "  function quotaPercent(item)")
	end := strings.Index(page, "  function bundleSummaryMetrics(results, hasQuotaWindows)")
	if start < 0 || end <= start {
		t.Fatal("cannot locate dashboard aggregation functions")
	}
	helpers := `
function owns(value, key) { return Object.prototype.hasOwnProperty.call(value || {}, key); }
function finiteNumber(value) {
  if (value == null || (typeof value === "string" && !value.trim())) return null;
  var number = Number(value);
  return Number.isFinite(number) ? number : null;
}
function clampPercent(value) { return Math.max(0, Math.min(100, Number(value) || 0)); }
function canonicalQuotaUnit(value) { return String(value || "").trim(); }
function translateWindowLabel(value) { return String(value || "").trim(); }
`
	script := helpers + page[start:end] + "\nprocess.stdout.write(JSON.stringify(aggregateQuotaWindows(" + results + ")));"
	output, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("execute dashboard aggregation: %v\n%s", err, output)
	}
	var windows []map[string]any
	if err := json.Unmarshal(output, &windows); err != nil {
		t.Fatalf("decode dashboard aggregation: %v\n%s", err, output)
	}
	return windows
}

func TestDashboardEmbeddedScriptHasValidSyntax(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	page := string(RenderDashboard(300))
	start := strings.LastIndex(page, "<script>")
	end := strings.LastIndex(page, "</script>")
	if start < 0 || end <= start {
		t.Fatal("cannot locate embedded dashboard script")
	}
	command := exec.Command(node, "--check", "-")
	command.Stdin = strings.NewReader(page[start+len("<script>") : end])
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("dashboard script syntax: %v\n%s", err, output)
	}
}

func findAggregatedWindow(t *testing.T, windows []map[string]any, label, unit string) map[string]any {
	t.Helper()
	for _, window := range windows {
		if window["label"] == label && window["unit"] == unit {
			return window
		}
	}
	t.Fatalf("missing aggregate window %q/%q in %#v", label, unit, windows)
	return nil
}

func numberField(t *testing.T, value map[string]any, key string) float64 {
	t.Helper()
	number, ok := value[key].(float64)
	if !ok {
		t.Fatalf("field %q is not numeric in %#v", key, value)
	}
	return number
}

func TestDashboardAggregationTracksMissingUnknownUnavailableAndFailedKeys(t *testing.T) {
	windows := runDashboardAggregation(t, `[
  {"fetched_at":"2099-07-28T00:00:00Z","quota_windows":[{"group":"通用模型","label":"每周配额","aggregation_key":"weekly","aggregation_scope":"key","unit":"次","total":100,"remaining":90,"remaining_percent":90,"reset_in_seconds":3600}]},
  {"fetched_at":"2099-07-28T00:00:00Z","quota_windows":[{"group":"通用模型","label":"每周配额","aggregation_key":"weekly","aggregation_scope":"key","unit":"次","total":1000,"remaining":10,"remaining_percent":1,"reset_in_seconds":7200}]},
  {"fetched_at":"2099-07-28T00:00:00Z","quota_windows":[{"group":"通用模型","label":"5 小时配额","aggregation_key":"short","aggregation_scope":"key","remaining_percent":80}]},
  {"fetched_at":"2099-07-28T00:00:00Z","quota_windows":[{"group":"通用模型","label":"每周配额","aggregation_key":"weekly","aggregation_scope":"key","unit":"次","unavailable":true}]},
  {"fetched_at":"2099-07-28T00:00:00Z","quota_windows":[{"group":"通用模型","label":"每周配额","aggregation_key":"weekly","aggregation_scope":"key","unit":"次","unknown":true}]},
  {"error":"认证失败","quota_windows":[]}
]`)
	weekly := findAggregatedWindow(t, windows, "每周配额", "次")
	for key, want := range map[string]float64{
		"aggregate_finite_count":      2,
		"aggregate_unavailable_count": 1,
		"aggregate_unknown_count":     1,
		"aggregate_missing_count":     1,
		"aggregate_failed_count":      1,
		"total":                       1100,
		"remaining":                   100,
	} {
		if got := numberField(t, weekly, key); got != want {
			t.Fatalf("%s = %v, want %v in %#v", key, got, want, weekly)
		}
	}
	if progress := numberField(t, weekly, "progress_remaining_percent"); math.Abs(progress-100.0/1100.0*100) > 0.001 {
		t.Fatalf("weighted progress = %v, want %v", progress, 100.0/1100.0*100)
	}
	if weekly["reset_staggered"] != true {
		t.Fatalf("staggered reset was not preserved: %#v", weekly)
	}
	if weekly["reset_partial"] != true {
		t.Fatalf("partial reset coverage was not preserved: %#v", weekly)
	}
	if weekly["status"] != "部分密钥未计入" {
		t.Fatalf("partial status = %#v, want 部分密钥未计入", weekly["status"])
	}
}

func TestDashboardAggregationUsesKnownResetFromUnknownWindow(t *testing.T) {
	windows := runDashboardAggregation(t, `[
  {"fetched_at":"2099-07-28T00:00:00Z","quota_windows":[{"group":"模型额度","label":"每周配额","aggregation_key":"weekly","aggregation_scope":"key","remaining_percent":50,"reset_at":"2099-08-03T00:00:00Z"}]},
  {"fetched_at":"2099-07-28T00:00:00Z","quota_windows":[{"group":"模型额度","label":"每周配额","aggregation_key":"weekly","aggregation_scope":"key","unknown":true,"reset_at":"2099-08-01T00:00:00Z"}]}
]`)
	window := findAggregatedWindow(t, windows, "每周配额", "")
	if window["reset_at"] != "2099-08-01T00:00:00.000Z" || window["reset_staggered"] != true {
		t.Fatalf("unknown-window reset was not included: %#v", window)
	}
}

func TestDashboardAggregationFallsBackFromStaleRelativeResetToAbsoluteReset(t *testing.T) {
	windows := runDashboardAggregation(t, `[
  {"fetched_at":"2020-01-01T00:00:00Z","quota_windows":[{"group":"模型额度","label":"每周配额","aggregation_key":"weekly","aggregation_scope":"key","remaining_percent":50,"reset_in_seconds":3600,"reset_at":"2099-08-03T00:00:00Z"}]}
]`)
	window := findAggregatedWindow(t, windows, "每周配额", "")
	if window["reset_at"] != "2099-08-03T00:00:00.000Z" {
		t.Fatalf("absolute reset fallback was lost: %#v", window)
	}
}

func TestDashboardAggregationDoesNotCallPartialExhaustedGroupFullyDepleted(t *testing.T) {
	windows := runDashboardAggregation(t, `[
  {"quota_windows":[{"group":"模型额度","label":"每周配额","aggregation_key":"weekly","aggregation_scope":"key","remaining_percent":0,"capacity_percent":100}]},
  {"error":"查询失败"}
]`)
	window := findAggregatedWindow(t, windows, "每周配额", "")
	if window["status"] != "部分密钥未计入" {
		t.Fatalf("partial depleted group status = %#v", window)
	}
}

func TestDashboardAggregationSeparatesUnitsAndPropagatesExplicitUnlimited(t *testing.T) {
	windows := runDashboardAggregation(t, `[
  {"quota_windows":[{"group":"模型额度","label":"每周配额","aggregation_key":"weekly","aggregation_scope":"key","unit":"次","total":100,"remaining":80}]},
  {"quota_windows":[{"group":"模型额度","label":"每周配额","aggregation_key":"weekly","aggregation_scope":"key","unit":"次","unlimited":true}]},
  {"quota_windows":[{"group":"模型额度","label":"每周配额","aggregation_key":"weekly","aggregation_scope":"key","unit":"令牌","total":1000,"remaining":500}]}
]`)
	requestWindow := findAggregatedWindow(t, windows, "每周配额", "次")
	if requestWindow["unlimited"] != true || numberField(t, requestWindow, "aggregate_unlimited_count") != 1 || numberField(t, requestWindow, "aggregate_finite_count") != 1 {
		t.Fatalf("explicit unlimited aggregation = %#v", requestWindow)
	}
	tokenWindow := findAggregatedWindow(t, windows, "每周配额", "令牌")
	if numberField(t, tokenWindow, "total") != 1000 || numberField(t, tokenWindow, "aggregate_missing_count") != 2 {
		t.Fatalf("unit-isolated aggregation = %#v", tokenWindow)
	}
}

func TestDashboardAggregationShowsPercentPoolCapacity(t *testing.T) {
	windows := runDashboardAggregation(t, `[
  {"quota_windows":[{"group":"模型额度","label":"5 小时配额","aggregation_key":"short","aggregation_scope":"key","remaining_percent":80,"capacity_percent":100}]},
  {"quota_windows":[{"group":"模型额度","label":"5 小时配额","aggregation_key":"short","aggregation_scope":"key","remaining_percent":135,"capacity_percent":150}]}
]`)
	window := findAggregatedWindow(t, windows, "5 小时配额", "")
	if numberField(t, window, "remaining_percent") != 215 || numberField(t, window, "capacity_percent") != 250 || numberField(t, window, "progress_remaining_percent") != 86 {
		t.Fatalf("percent capacity pool = %#v", window)
	}
}

func TestDashboardAggregationDoesNotSumDifferentAccountScopedValues(t *testing.T) {
	windows := runDashboardAggregation(t, `[
  {"quota_windows":[{"group":"订阅套餐","label":"每周额度","aggregation_key":"weekly","aggregation_scope":"account","remaining_percent":80,"capacity_percent":100}]},
  {"quota_windows":[{"group":"订阅套餐","label":"每周额度","aggregation_key":"weekly","aggregation_scope":"account","remaining_percent":35,"capacity_percent":100}]}
]`)
	window := findAggregatedWindow(t, windows, "每周额度", "")
	if window["aggregate_range"] != true || numberField(t, window, "range_min_percent") != 35 || numberField(t, window, "range_max_percent") != 80 {
		t.Fatalf("account-scoped range = %#v", window)
	}
	if _, summed := window["remaining_percent"]; summed {
		t.Fatalf("account-scoped values were incorrectly summed: %#v", window)
	}
}

func TestDashboardAggregationMarksPartialAccountCoverage(t *testing.T) {
	windows := runDashboardAggregation(t, `[
  {"quota_windows":[{"group":"订阅套餐","label":"每周额度","aggregation_key":"weekly","aggregation_scope":"account","remaining_percent":80,"capacity_percent":100}]},
  {"quota_windows":[]},
  {"error":"查询失败"}
]`)
	window := findAggregatedWindow(t, windows, "每周额度", "")
	if window["status"] != "部分账户未计入" || numberField(t, window, "aggregate_missing_count") != 1 || numberField(t, window, "aggregate_failed_count") != 1 {
		t.Fatalf("partial account coverage = %#v", window)
	}
}

func TestDashboardAggregationDoesNotCopyAccountAmountsWithOnlyEqualPercent(t *testing.T) {
	windows := runDashboardAggregation(t, `[
  {"quota_windows":[{"group":"订阅套餐","label":"每周额度","aggregation_key":"weekly","aggregation_scope":"account","unit":"次","total":100,"remaining":50}]},
  {"quota_windows":[{"group":"订阅套餐","label":"每周额度","aggregation_key":"weekly","aggregation_scope":"account","unit":"次","total":200,"remaining":100}]}
]`)
	window := findAggregatedWindow(t, windows, "每周额度", "次")
	if numberField(t, window, "remaining_percent") != 50 || window["status"] != "账户级剩余比例一致" {
		t.Fatalf("account-scoped common percent = %#v", window)
	}
	if _, exists := window["total"]; exists {
		t.Fatalf("different account totals were copied as an exact aggregate: %#v", window)
	}
	if _, exists := window["remaining"]; exists {
		t.Fatalf("different account remaining amounts were copied as an exact aggregate: %#v", window)
	}
}

func TestDashboardAggregationComparesNormalizedAccountPercentages(t *testing.T) {
	windows := runDashboardAggregation(t, `[
  {"quota_windows":[{"group":"订阅套餐","label":"每周额度","aggregation_key":"weekly","aggregation_scope":"account","remaining_percent":80,"capacity_percent":100}]},
  {"quota_windows":[{"group":"订阅套餐","label":"每周额度","aggregation_key":"weekly","aggregation_scope":"account","remaining_percent":80,"capacity_percent":150}]}
]`)
	window := findAggregatedWindow(t, windows, "每周额度", "")
	if window["aggregate_range"] != true {
		t.Fatalf("different normalized account percentages were treated as equal: %#v", window)
	}
}

func TestDashboardAggregationNormalizesBoostedSingleKeyProgress(t *testing.T) {
	windows := runDashboardAggregation(t, `[
  {"quota_windows":[{"group":"模型额度","label":"4 小时配额","aggregation_key":"short","aggregation_scope":"key","remaining_percent":135,"capacity_percent":150}]}
]`)
	window := findAggregatedWindow(t, windows, "4 小时配额", "")
	if numberField(t, window, "progress_remaining_percent") != 90 {
		t.Fatalf("boosted progress = %#v", window)
	}
}

func TestDashboardCardProgressNormalizesBoostedCapacity(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	page := string(RenderDashboard(300))
	start := strings.Index(page, "  function quotaPercent(item)")
	end := strings.Index(page, "  function bundleSummaryMetrics(results, hasQuotaWindows)")
	if start < 0 || end <= start {
		t.Fatal("cannot locate dashboard quota helpers")
	}
	helpers := `
function owns(value, key) { return Object.prototype.hasOwnProperty.call(value || {}, key); }
function finiteNumber(value) {
  if (value == null || (typeof value === "string" && !value.trim())) return null;
  var number = Number(value);
  return Number.isFinite(number) ? number : null;
}
function clampPercent(value) { return Math.max(0, Math.min(100, Number(value) || 0)); }
function canonicalQuotaUnit(value) { return String(value || "").trim(); }
function translateWindowLabel(value) { return String(value || "").trim(); }
`
	script := helpers + page[start:end] + `
process.stdout.write(JSON.stringify(quotaProgressRemainingPercent({remaining_percent:135,capacity_percent:150})));
`
	output, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("execute dashboard progress helper: %v\n%s", err, output)
	}
	var progress float64
	if err := json.Unmarshal(output, &progress); err != nil {
		t.Fatalf("decode dashboard progress: %v\n%s", err, output)
	}
	if progress != 90 {
		t.Fatalf("boosted card progress = %v, want 90", progress)
	}
}

func TestDashboardAggregationSumsRemainingOnlyKeyValuesIncludingZero(t *testing.T) {
	windows := runDashboardAggregation(t, `[
  {"quota_windows":[{"group":"钱包","label":"剩余额度","aggregation_key":"remaining","aggregation_scope":"key","unit":"USD","remaining":10}]},
  {"quota_windows":[{"group":"钱包","label":"剩余额度","aggregation_key":"remaining","aggregation_scope":"key","unit":"USD","remaining":0}]}
]`)
	window := findAggregatedWindow(t, windows, "剩余额度", "USD")
	if numberField(t, window, "remaining") != 10 || window["aggregate_remaining"] != true {
		t.Fatalf("remaining-only key aggregate = %#v", window)
	}
	if _, exists := window["total"]; exists {
		t.Fatalf("remaining-only aggregation invented a total: %#v", window)
	}
}

func TestDashboardAggregationDoesNotPromoteMixedAccountQuotaToUnlimited(t *testing.T) {
	windows := runDashboardAggregation(t, `[
  {"quota_windows":[{"group":"订阅套餐","label":"每周额度","aggregation_key":"weekly","aggregation_scope":"account","remaining_percent":80,"capacity_percent":100}]},
  {"quota_windows":[{"group":"订阅套餐","label":"每周额度","aggregation_key":"weekly","aggregation_scope":"account","unlimited":true}]}
]`)
	window := findAggregatedWindow(t, windows, "每周额度", "")
	if window["aggregate_mixed_unlimited"] != true || window["status"] != "有限额与不限量并存" {
		t.Fatalf("mixed account quota = %#v", window)
	}
	if window["unlimited"] == true {
		t.Fatalf("mixed account quota was incorrectly promoted to unlimited: %#v", window)
	}
}

func TestDashboardAggregationDoesNotPromotePartialAccountUnlimitedToWholeGroup(t *testing.T) {
	windows := runDashboardAggregation(t, `[
  {"quota_windows":[{"group":"订阅套餐","label":"每周额度","aggregation_key":"weekly","aggregation_scope":"account","unlimited":true}]},
  {"quota_windows":[{"group":"订阅套餐","label":"每周额度","aggregation_key":"weekly","aggregation_scope":"account","unavailable":true}]}
]`)
	window := findAggregatedWindow(t, windows, "每周额度", "")
	if window["aggregate_mixed_unlimited"] != true || window["status"] != "部分账户不限量" {
		t.Fatalf("partial account unlimited = %#v", window)
	}
	if window["unlimited"] == true {
		t.Fatalf("partial account unlimited was incorrectly promoted to whole group: %#v", window)
	}
}
