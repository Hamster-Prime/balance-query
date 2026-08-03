package ui

import (
	"encoding/json"
	"math"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func dashboardQuotaHelpers(t *testing.T) string {
	t.Helper()
	page := string(RenderDashboard(300))
	start := strings.Index(page, "  function quotaPercent(item)")
	end := strings.Index(page, "  function bundleSummaryMetrics(results, hasQuotaWindows)")
	if start < 0 || end <= start {
		t.Fatal("cannot locate dashboard quota functions")
	}
	return `
function owns(value, key) { return Object.prototype.hasOwnProperty.call(value || {}, key); }
function finiteNumber(value) {
  if (value == null || (typeof value === "string" && !value.trim())) return null;
  var number = Number(value);
  return Number.isFinite(number) ? number : null;
}
function clampPercent(value) { return Math.max(0, Math.min(100, Number(value) || 0)); }
function canonicalQuotaUnit(value) { return String(value || "").trim(); }
function translateWindowLabel(value) { return String(value || "").trim(); }
` + page[start:end]
}

func runDashboardAggregation(t *testing.T, results string) []map[string]any {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	script := dashboardQuotaHelpers(t) + "\nprocess.stdout.write(JSON.stringify(aggregateQuotaWindows(" + results + ")));"
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

func runDashboardQuotaResultState(t *testing.T, windows string) string {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	script := dashboardQuotaHelpers(t) + "\nprocess.stdout.write(quotaResultState(" + windows + "));"
	output, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("execute dashboard quota result state: %v\n%s", err, output)
	}
	return string(output)
}

func runDashboardQuotaGroupOrder(t *testing.T, groups string) []string {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	script := dashboardQuotaHelpers(t) + "\nprocess.stdout.write(JSON.stringify(orderedQuotaGroups(" + groups + ").map(function (group) { return group.name; })));"
	output, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("execute dashboard quota group ordering: %v\n%s", err, output)
	}
	var names []string
	if err := json.Unmarshal(output, &names); err != nil {
		t.Fatalf("decode dashboard quota group ordering: %v\n%s", err, output)
	}
	return names
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

func TestDashboardDistinguishesPartialAndCompleteQuotaExhaustion(t *testing.T) {
	tests := []struct {
		name    string
		windows string
		want    string
	}{
		{
			name: "one model exhausted while another remains",
			windows: `[
  {"group":"通用模型","remaining_percent":80,"capacity_percent":100},
  {"group":"视频模型","total":3,"used":3,"remaining_percent":0,"status":"已用尽"}
]`,
			want: "partial-exhausted",
		},
		{
			name: "all finite quotas exhausted",
			windows: `[
  {"group":"通用模型","remaining_percent":0,"used_percent":100,"status":"已用尽"},
  {"group":"视频模型","remaining_percent":0,"used_percent":100,"status":"已用尽"}
]`,
			want: "all-exhausted",
		},
		{
			name:    "exhausted plus incomplete is not definitive",
			windows: `[{"remaining_percent":0,"used_percent":100,"status":"已用尽"},{"unknown":true}]`,
			want:    "incomplete",
		},
		{
			name:    "unlimited resource keeps result partially available",
			windows: `[{"remaining_percent":0,"used_percent":100,"status":"已用尽"},{"unlimited":true}]`,
			want:    "partial-exhausted",
		},
		{
			name:    "unavailable resource is neutral",
			windows: `[{"remaining_percent":80,"capacity_percent":100},{"unavailable":true}]`,
			want:    "normal",
		},
		{
			name:    "exhausted blocking window makes sibling capacity unusable",
			windows: `[{"group":"滚动限流","label":"5 小时配额","blocking":true,"remaining_percent":80,"capacity_percent":100},{"group":"订阅配额","label":"每周配额","blocking":true,"remaining_percent":0,"used_percent":100,"status":"已用尽"},{"group":"加量包","label":"加量包余额","total":200,"remaining":100,"status":"Extra Usage 开关未知"}]`,
			want:    "blocked",
		},
		{
			name:    "optional exhausted booster does not limit healthy base quotas",
			windows: `[{"group":"滚动限流","label":"5 小时配额","blocking":true,"remaining_percent":80,"capacity_percent":100},{"group":"订阅配额","label":"每周配额","blocking":true,"remaining_percent":60,"capacity_percent":100},{"group":"加量包","label":"加量包余额","total":200,"remaining":0,"status":"余额已用尽"}]`,
			want:    "normal",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := runDashboardQuotaResultState(t, test.windows); got != test.want {
				t.Fatalf("quota result state = %q, want %q", got, test.want)
			}
		})
	}
}

func TestDashboardTreatsFreshlyResetKimiQuotasAsNormal(t *testing.T) {
	windows := `[
  {"group":"订阅配额","label":"每周配额","blocking":true,"used_percent":0,"remaining_percent":100,"capacity_percent":100},
  {"group":"滚动限流","label":"5 小时配额","blocking":true,"used_percent":0,"remaining_percent":100,"capacity_percent":100}
]`
	if got := runDashboardQuotaResultState(t, windows); got != "normal" {
		t.Fatalf("fresh Kimi quota result state = %q, want normal", got)
	}
}

func TestDashboardShowsKimiRollingLimitBeforeWeeklyQuota(t *testing.T) {
	got := runDashboardQuotaGroupOrder(t, `[
  {"name":"订阅配额","order":0},
  {"name":"加量包","order":1},
  {"name":"滚动限流","order":2}
]`)
	want := []string{"滚动限流", "订阅配额", "加量包"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Kimi quota group order = %#v, want %#v", got, want)
	}
}

func TestDashboardAggregationPreservesKimiBlockingSemantics(t *testing.T) {
	windows := runDashboardAggregation(t, `[
  {"quota_windows":[
    {"group":"滚动限流","label":"5 小时配额","aggregation_key":"kimi:5 小时配额","aggregation_scope":"account","blocking":true,"remaining_percent":80,"capacity_percent":100},
    {"group":"订阅配额","label":"每周配额","aggregation_key":"kimi:每周配额","aggregation_scope":"account","blocking":true,"remaining_percent":0,"used_percent":100,"capacity_percent":100,"status":"已用尽"},
    {"group":"加量包","label":"加量包余额","aggregation_key":"kimi:booster-balance","aggregation_scope":"account","unit":"USD","total":200,"remaining":100,"status":"Extra Usage 开关未知"}
  ]}
]`)
	weekly := findAggregatedWindow(t, windows, "每周配额", "")
	if weekly["blocking"] != true {
		t.Fatalf("aggregated Kimi weekly quota lost blocking semantics: %#v", weekly)
	}
	encoded, err := json.Marshal(windows)
	if err != nil {
		t.Fatalf("encode aggregated Kimi windows: %v", err)
	}
	if got := runDashboardQuotaResultState(t, string(encoded)); got != "blocked" {
		t.Fatalf("aggregated Kimi quota state = %q, want blocked; windows = %#v", got, windows)
	}
}

func TestDashboardAggregationTreatsUnavailableAsKnownPlanCoverage(t *testing.T) {
	windows := runDashboardAggregation(t, `[
  {"quota_windows":[{"group":"视频模型","label":"每日配额","aggregation_key":"video-daily","aggregation_scope":"key","unit":"次","total":3,"remaining":2}]},
  {"quota_windows":[{"group":"视频模型","label":"每日配额","aggregation_key":"video-daily","aggregation_scope":"key","unit":"次","unavailable":true,"status":"不在当前套餐中"}]}
]`)
	window := findAggregatedWindow(t, windows, "每日配额", "次")
	if window["status"] != "部分密钥的套餐未提供此项" || numberField(t, window, "aggregate_unavailable_count") != 1 {
		t.Fatalf("known unavailable coverage = %#v", window)
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

func TestDashboardSessionSnapshotIsHashedRedactedAndTTLBound(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	page := string(RenderDashboard(300))
	start := strings.Index(page, "  function utf8Bytes(value)")
	end := strings.Index(page, "  function deobfuscate(raw)")
	sourceStart := strings.Index(page, "  function canonicalProviderSource(value, category)")
	sourceEnd := strings.Index(page, "  function providerCategoryRank(value)")
	matchStart := strings.Index(page, "  function resultMatchesAccount(result, account)")
	matchEnd := strings.Index(page, "  function resultsForAccounts(results, accounts)")
	if start < 0 || end <= start || sourceStart < 0 || sourceEnd <= sourceStart || matchStart < 0 || matchEnd <= matchStart {
		t.Fatal("cannot locate dashboard snapshot helpers")
	}
	script := `
var QUERY_BATCH_SIZE = 128;
var RESULT_SNAPSHOT_KEY = "balance-query-results::v1";
var RESULT_SNAPSHOT_VERSION = 2;
var RESULT_SNAPSHOT_MAX_BYTES = 2 * 1024 * 1024;
var RESULT_SNAPSHOT_MAX_STALE_MS = 7 * 24 * 60 * 60 * 1000;
var SECRET_SALT = "cli-proxy-api-webui::secure-storage";
var state = {
  snapshotGeneration:0,
  providers:[{name:"服务一",baseUrl:"https://provider.example/v1",category:"OpenAI 兼容",source:"openai-compatibility",disabled:false,mappingKey:"provider-1",legacyMappingKey:"provider-1",keys:[]}],
  config:{cache_ttl_seconds:300,provider_mappings:{"provider-1":"newapi"}}
};
var memory = Object.create(null);
var storage = {
  getItem:function (key) { return Object.prototype.hasOwnProperty.call(memory, key) ? memory[key] : null; },
  setItem:function (key, value) { memory[key] = String(value); },
  removeItem:function (key) { delete memory[key]; }
};
var window = { crypto:null, sessionStorage:storage, location:{origin:"https://cpa.example",host:"cpa.example"} };
window.parent = window;
function maskKey(value) { return "masked-" + String(value).slice(-2); }
function providerCategoryLabel(value) { return String(value || "OpenAI 兼容"); }
function normalizeBaseForKey(value) { return String(value || "").replace(/\/+$/, ""); }
function normalizeConfig(value) { return {cache_ttl_seconds:Number(value.cache_ttl_seconds),provider_mappings:Object.assign({},value.provider_mappings || {})}; }
var now = Date.parse("2099-07-29T00:00:00Z");
Date.now = function () { return now; };
` + page[start:end] + page[sourceStart:sourceEnd] + page[matchStart:matchEnd] + `
(async function () {
  var credentials = {apiBase:"https://cpa.example",managementKey:"management-secret"};
  var accounts = [{
    id:"account-1",provider_key:"provider-1",mapping_key:"provider-1",account_name:"账户一",
    base_url:"https://provider.example/v1",proxy_url:"",query_type:"newapi",api_key:"sk-provider-secret",source:"openai-compatibility"
  }];
  var results = [attachResultAccountDigest({
    auth_id:"account-1",provider_key:"provider-1",base_url:"https://provider.example/v1",
    account_name:"账户一",fetched_at:"2099-07-29T00:00:00Z",extra:{note:"sk-provider-secret / management-secret"}
  }, accountResultDigest(accounts[0]))];
  var wrote = await persistResultSnapshot(accounts, results, 300, credentials);
  var serialized = storage.getItem(RESULT_SNAPSHOT_KEY) || "";
  var legacyRecord = JSON.parse(serialized);
  delete legacyRecord.display_providers[0].source;
  delete legacyRecord.display_accounts[0].source;
  storage.setItem(RESULT_SNAPSHOT_KEY, JSON.stringify(legacyRecord));
  var legacyPreview = await readResultSnapshotPreview(credentials);
  var restored = await readResultSnapshot(accounts, 300, credentials);
  var warningResults = [attachResultAccountDigest({
    auth_id:"account-1",provider_key:"provider-1",base_url:"https://provider.example/v1",
    account_name:"账户一",fetched_at:"2099-07-29T00:00:00Z",balance:12.5,
    warnings:[{kind:"service_unavailable",title:"费用数据暂缺",reason:"sk-provider-secret / management-secret",suggestion:"稍后重试"}]
  }, accountResultDigest(accounts[0]))];
  await persistResultSnapshot(accounts, warningResults, 300, credentials);
  var warningSerialized = storage.getItem(RESULT_SNAPSHOT_KEY) || "";
  var warningRecord = JSON.parse(warningSerialized);
  var warningRestored = await readResultSnapshot(accounts, 300, credentials);
  await persistResultSnapshot(accounts, results, 300, credentials);
  now += 301000;
  var stale = await readResultSnapshot(accounts, 300, credentials);
  var changed = JSON.parse(JSON.stringify(accounts));
  changed[0].api_key = "sk-provider-changed";
  var mismatch = await readResultSnapshot(changed, 300, credentials);
  var secondAccount = {
    id:"account-2",provider_key:"provider-2",mapping_key:"provider-2",account_name:"账户二",
    base_url:"https://second.example/v1",proxy_url:"",query_type:"deepseek",api_key:"sk-second-provider-secret",source:"claude-api-key"
  };
  var orderedAccounts = [accounts[0], secondAccount];
  var orderedResults = [
    attachResultAccountDigest({auth_id:"account-1",provider_key:"provider-1",base_url:"https://provider.example/v1",account_name:"账户一",fetched_at:"2099-07-29T00:00:00Z",extra:{marker:"first"}}, accountResultDigest(accounts[0])),
    attachResultAccountDigest({auth_id:"account-2",provider_key:"provider-2",base_url:"https://second.example/v1",account_name:"账户二",fetched_at:"2099-07-29T00:00:00Z",extra:{marker:"second"}}, accountResultDigest(secondAccount))
  ];
  await persistResultSnapshot(orderedAccounts, [orderedResults[1], orderedResults[0]], 300, credentials);
  var storedOrderedMarkers = JSON.parse(storage.getItem(RESULT_SNAPSHOT_KEY)).results.map(function (result) { return result.extra.marker; });
  var reordered = await readResultSnapshot([secondAccount, accounts[0]], 300, credentials);
  var preview = await readResultSnapshotPreview(credentials);
  var changedQuery = JSON.parse(JSON.stringify(accounts[0]));
  changedQuery.query_type = "deepseek";
  var changedProxy = JSON.parse(JSON.stringify(accounts[0]));
  changedProxy.proxy_url = "http://proxy.example:8080";
  await persistResultSnapshot(accounts, results, 300, credentials);
  var ttlZero = await readResultSnapshot(accounts, 0, credentials);
  state.providers[0].name = "写入开始前";
  state.config.provider_mappings = {"provider-1":"newapi"};
  var pendingCapture = persistResultSnapshot(accounts, results, 300, credentials);
  state.providers = [{name:"异步期间的新状态",baseUrl:"https://new.example",category:"其他",mappingKey:"new",legacyMappingKey:"new",keys:[]}];
  state.config.provider_mappings = {new:"deepseek"};
  await pendingCapture;
  var capturedRecord = JSON.parse(storage.getItem(RESULT_SNAPSHOT_KEY));
  process.stdout.write(JSON.stringify({
    wrote:wrote,
    contains_api_key:serialized.indexOf("sk-provider-secret") !== -1,
    contains_management_key:serialized.indexOf("management-secret") !== -1,
    restored_fresh:restored && restored.fresh,
    restored_complete:restored && restored.complete,
    restored_note:restored && restored.results[0].extra.note,
    warning_contains_api_key:warningSerialized.indexOf("sk-provider-secret") !== -1,
    warning_contains_management_key:warningSerialized.indexOf("management-secret") !== -1,
    warning_fresh:warningRestored && warningRestored.fresh,
    warning_complete:warningRestored && warningRestored.complete,
    warning_count:warningRestored && warningRestored.results[0].warnings.length,
    warning_reason:warningRestored && warningRestored.results[0].warnings[0].reason,
    warning_expires_in:warningRecord.expires_at - warningRecord.stored_at,
    stale_fresh:stale && stale.fresh,
    mismatch:mismatch,
    stored_ordered_markers:storedOrderedMarkers,
    reordered_markers:reordered && reordered.results.map(function (result) { return result.extra.marker; }),
    preview_accounts:preview && preview.accounts.length,
    preview_digest_matches:preview && accountResultDigest(preview.accounts[0]) === accountResultDigest(accounts[0]) && preview.results[0].__account_digest === accountResultDigest(accounts[0]),
    preview_rejects_api_key:preview && accountResultDigest(preview.accounts[0]) !== accountResultDigest(changed[0]),
    preview_rejects_query:preview && accountResultDigest(preview.accounts[0]) !== accountResultDigest(changedQuery),
    preview_rejects_proxy:preview && accountResultDigest(preview.accounts[0]) !== accountResultDigest(changedProxy),
    legacy_provider_source:legacyPreview && legacyPreview.providers[0].source,
    legacy_account_source:legacyPreview && legacyPreview.accounts[0].source,
    ttl_zero_fresh:ttlZero && ttlZero.fresh,
    retained:storage.getItem(RESULT_SNAPSHOT_KEY) != null,
    captured_provider:capturedRecord.display_providers[0].name,
    captured_mapping:capturedRecord.config.provider_mappings["provider-1"],
    captured_provider_source:capturedRecord.display_providers[0].source,
    captured_account_source:capturedRecord.display_accounts[0].source,
    fallback_sha:fallbackSHA256("abc")
  }));
})().catch(function (error) { process.stderr.write(String(error && error.stack || error)); process.exit(1); });
`
	output, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("execute dashboard snapshot helpers: %v\n%s", err, output)
	}
	var got struct {
		Wrote                  bool     `json:"wrote"`
		ContainsAPIKey         bool     `json:"contains_api_key"`
		ContainsManagementKey  bool     `json:"contains_management_key"`
		RestoredFresh          bool     `json:"restored_fresh"`
		RestoredComplete       bool     `json:"restored_complete"`
		RestoredNote           string   `json:"restored_note"`
		WarningContainsAPIKey  bool     `json:"warning_contains_api_key"`
		WarningContainsMgmt    bool     `json:"warning_contains_management_key"`
		WarningFresh           bool     `json:"warning_fresh"`
		WarningComplete        bool     `json:"warning_complete"`
		WarningCount           int      `json:"warning_count"`
		WarningReason          string   `json:"warning_reason"`
		WarningExpiresIn       int64    `json:"warning_expires_in"`
		StaleFresh             bool     `json:"stale_fresh"`
		Mismatch               any      `json:"mismatch"`
		StoredOrderedMarkers   []string `json:"stored_ordered_markers"`
		ReorderedMarkers       []string `json:"reordered_markers"`
		PreviewAccounts        int      `json:"preview_accounts"`
		PreviewDigestMatches   bool     `json:"preview_digest_matches"`
		PreviewRejectsAPIKey   bool     `json:"preview_rejects_api_key"`
		PreviewRejectsQuery    bool     `json:"preview_rejects_query"`
		PreviewRejectsProxy    bool     `json:"preview_rejects_proxy"`
		LegacyProviderSource   string   `json:"legacy_provider_source"`
		LegacyAccountSource    string   `json:"legacy_account_source"`
		TTLZeroFresh           bool     `json:"ttl_zero_fresh"`
		Retained               bool     `json:"retained"`
		CapturedProvider       string   `json:"captured_provider"`
		CapturedMapping        string   `json:"captured_mapping"`
		CapturedProviderSource string   `json:"captured_provider_source"`
		CapturedAccountSource  string   `json:"captured_account_source"`
		FallbackSHA            string   `json:"fallback_sha"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode dashboard snapshot result: %v\n%s", err, output)
	}
	if !got.Wrote || got.ContainsAPIKey || got.ContainsManagementKey {
		t.Fatalf("unsafe snapshot persistence result: %#v", got)
	}
	if !got.RestoredFresh || !got.RestoredComplete || strings.Contains(got.RestoredNote, "secret") {
		t.Fatalf("fresh snapshot was not safely restored: %#v", got)
	}
	if !got.WarningFresh || got.WarningComplete || got.WarningCount != 1 || got.WarningContainsAPIKey || got.WarningContainsMgmt || strings.Contains(got.WarningReason, "secret") {
		t.Fatalf("partial warning snapshot was not safely restored for background retry: %#v", got)
	}
	if got.WarningExpiresIn != 20_000 {
		t.Fatalf("warning snapshot expiry = %d, want 20 seconds", got.WarningExpiresIn)
	}
	if got.StaleFresh || got.TTLZeroFresh || got.Mismatch != nil || !got.Retained {
		t.Fatalf("snapshot TTL/fingerprint invalidation failed: %#v", got)
	}
	if len(got.StoredOrderedMarkers) != 2 || got.StoredOrderedMarkers[0] != "first" || got.StoredOrderedMarkers[1] != "second" ||
		len(got.ReorderedMarkers) != 2 || got.ReorderedMarkers[0] != "second" || got.ReorderedMarkers[1] != "first" || got.PreviewAccounts != 2 {
		t.Fatalf("snapshot did not restore display data across account ordering: %#v", got)
	}
	if !got.PreviewDigestMatches || !got.PreviewRejectsAPIKey || !got.PreviewRejectsQuery || !got.PreviewRejectsProxy {
		t.Fatalf("preview account digest did not preserve the full query identity: %#v", got)
	}
	if got.LegacyProviderSource != "openai-compatibility" || got.LegacyAccountSource != "openai-compatibility" {
		t.Fatalf("legacy snapshot provider source was not inferred: %#v", got)
	}
	if got.CapturedProvider != "写入开始前" || got.CapturedMapping != "newapi" {
		t.Fatalf("snapshot mixed async state: %#v", got)
	}
	if got.CapturedProviderSource != "openai-compatibility" || got.CapturedAccountSource != "openai-compatibility" {
		t.Fatalf("snapshot did not persist provider source metadata: %#v", got)
	}
	if got.FallbackSHA != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("fallback SHA-256 = %q", got.FallbackSHA)
	}
}

func TestDashboardOverviewGroupsReorderedResultsByMatchingAccount(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	page := string(RenderDashboard(300))
	start := strings.Index(page, "  function providerForKey(providerKey)")
	end := strings.Index(page, "  function canonicalQuotaUnit(unit)")
	if start < 0 || end <= start {
		t.Fatal("cannot locate dashboard overview grouping helpers")
	}
	script := `
var providers = [
  {name:"服务 A",baseUrl:"https://shared.example/v1",category:"OpenAI 兼容",mappingKey:"provider-a",legacyMappingKey:"provider-a"},
  {name:"服务 B",baseUrl:"https://shared.example/v1",category:"OpenAI 兼容",mappingKey:"provider-b",legacyMappingKey:"provider-b"}
];
var state = {
  config:{provider_mappings:{"provider-a":"fallback-a","provider-b":"fallback-b"}},
  results:[
    {marker:"second",auth_id:"account-b",provider_key:"provider-b",base_url:"https://shared.example/v1",__account_digest:"digest-b"},
    {marker:"first",auth_id:"account-a",provider_key:"provider-a",base_url:"https://shared.example/v1",__account_digest:"digest-a"}
  ]
};
function displayProviders() { return providers; }
function providerCategoryLabel(value) { return String(value || ""); }
function normalizeBaseForKey(value) { return String(value || "").replace(/\/+$/, ""); }
function mappedQueryType(provider, mappings) { return mappings[provider.mappingKey] || ""; }
function providerCategoryRank() { return 0; }
function serviceName(category, baseURL) { return category + " · " + baseURL; }
function resultWarnings() { return []; }
function resultMatchesAccount(result, account) {
  return result.__account_digest === account.__account_digest && result.auth_id === account.id && result.provider_key === account.provider_key;
}
` + page[start:end] + `
var accounts = [
  {id:"account-a",provider_key:"provider-a",query_type:"newapi",__account_digest:"digest-a"},
  {id:"account-b",provider_key:"provider-b",query_type:"deepseek",__account_digest:"digest-b"}
];
var assignments = {};
overviewResultGroups(accounts)[0].services.forEach(function (service) {
  service.results.forEach(function (result) { assignments[result.marker] = service.provider.queryType; });
});
process.stdout.write(JSON.stringify(assignments));
`
	output, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("execute dashboard reordered grouping: %v\n%s", err, output)
	}
	var got map[string]string
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode dashboard reordered grouping: %v\n%s", err, output)
	}
	if got["first"] != "newapi" || got["second"] != "deepseek" {
		t.Fatalf("reordered results used positional account metadata: %#v", got)
	}
}

func TestDashboardReconnectRequiresConfirmationForDirtyDraft(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	page := string(RenderDashboard(300))
	start := strings.Index(page, "  function confirmDiscardSettingsDraft()")
	end := strings.Index(page, "  function cancelLoadRequests()")
	if start < 0 || end <= start {
		t.Fatal("cannot locate dashboard reconnect confirmation helper")
	}
	if !strings.Contains(page, `if (!confirmDiscardSettingsDraft()) return;`) {
		t.Fatal("reconnect action does not stop when dirty-draft confirmation is denied")
	}
	script := `
var state = {dirty:false};
var decisions = [false, true];
var messages = [];
var window = {confirm:function (message) { messages.push(message); return decisions.shift(); }};
` + page[start:end] + `
var clean = confirmDiscardSettingsDraft();
state.dirty = true;
var denied = confirmDiscardSettingsDraft();
var accepted = confirmDiscardSettingsDraft();
process.stdout.write(JSON.stringify({clean:clean,denied:denied,accepted:accepted,messages:messages}));
`
	output, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("execute dashboard reconnect confirmation: %v\n%s", err, output)
	}
	var got struct {
		Clean    bool     `json:"clean"`
		Denied   bool     `json:"denied"`
		Accepted bool     `json:"accepted"`
		Messages []string `json:"messages"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode dashboard reconnect confirmation: %v\n%s", err, output)
	}
	if !got.Clean || got.Denied || !got.Accepted || len(got.Messages) != 2 || !strings.Contains(got.Messages[0], "未保存") {
		t.Fatalf("dirty reconnect confirmation = %#v", got)
	}
}

func TestDashboardQueryBatchesPreserveOrderAndIgnoreSupersededResults(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	page := string(RenderDashboard(300))
	start := strings.Index(page, "  function queryTimeoutForBatch(accountCount)")
	end := strings.Index(page, "  function keyChips(provider)")
	if start < 0 || end <= start {
		t.Fatal("cannot locate dashboard query helpers")
	}
	script := `
var QUERY_BATCH_SIZE = 128;
var QUERY_BATCH_MAX_BYTES = 900 * 1024;
var state = {
  credentials:{apiBase:"https://cpa.example",managementKey:"management-secret"},
  config:{cache_ttl_seconds:300},results:[{id:"visible-old"}],needsQuery:false,
  previewAccounts:[],queryController:null,queryGeneration:0,querying:false
};
var currentAccounts = [{id:"request-old"}];
var deferred = [];
var buttonStates = [];
var persisted = [];
var resultNode = { setAttribute:function () {} };
function buildAccounts() { return currentAccounts.slice(); }
function resultMatchesAccount() { return false; }
function accountResultDigest(account) { return String(account && account.id || ""); }
function attachResultAccountDigest(result, digest) { Object.defineProperty(result, "__account_digest", {value:String(digest),configurable:true}); return result; }
function appendUnavailablePreviewResults(results) { return results; }
function serializedByteLength(value) { return Buffer.byteLength(value, "utf8"); }
function byID(id) { return id === "results" ? resultNode : {}; }
function setButtonBusy(_button, busy) { buttonStates.push(Boolean(busy)); }
function showSkeletons() {}
function setText() {}
function renderResults() {}
function clearResultSnapshot() {}
function toast(message) { throw new Error("unexpected toast: " + message); }
function persistResultSnapshot(_accounts, results) { persisted.push(results.map(function (item) { return item.id; })); return Promise.resolve(true); }
function apiFetch(_path, options, timeout) {
  return new Promise(function (resolve, reject) { deferred.push({resolve:resolve,reject:reject,options:options,timeout:timeout}); });
}
` + page[start:end] + `
(async function () {
  var sizes = accountBatches(Array.from({length:260}, function (_, index) { return {id:index}; })).map(function (batch) { return batch.length; });
  var largeSizes = accountBatches(Array.from({length:3}, function (_, index) { return {id:index,api_key:"x".repeat(500000)}; })).map(function (batch) { return batch.length; });
  var first = queryBalances(false);
  while (deferred.length < 1) await Promise.resolve();
  currentAccounts = [{id:"request-new"}];
  var second = queryBalances(false);
  while (deferred.length < 2) await Promise.resolve();
  var firstAborted = deferred[0].options.signal.aborted;
  deferred[1].resolve({results:[{id:"result-new",fetched_at:"2099-07-29T00:00:00Z"}]});
  await second;
  deferred[0].resolve({results:[{id:"result-old",fetched_at:"2099-07-28T00:00:00Z"}]});
  await first;
  process.stdout.write(JSON.stringify({
    sizes:sizes,
    large_sizes:largeSizes,
    timeout_1:queryTimeoutForBatch(1),
    timeout_128:queryTimeoutForBatch(128),
    first_aborted:firstAborted,
    result_ids:state.results.map(function (item) { return item.id; }),
    persisted:persisted,
    false_busy_count:buttonStates.filter(function (busy) { return !busy; }).length,
    querying:state.querying
  }));
})().catch(function (error) { process.stderr.write(String(error && error.stack || error)); process.exit(1); });
`
	output, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("execute dashboard query helpers: %v\n%s", err, output)
	}
	var got struct {
		Sizes          []int      `json:"sizes"`
		LargeSizes     []int      `json:"large_sizes"`
		Timeout1       int        `json:"timeout_1"`
		Timeout128     int        `json:"timeout_128"`
		FirstAborted   bool       `json:"first_aborted"`
		ResultIDs      []string   `json:"result_ids"`
		Persisted      [][]string `json:"persisted"`
		FalseBusyCount int        `json:"false_busy_count"`
		Querying       bool       `json:"querying"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode dashboard query result: %v\n%s", err, output)
	}
	if len(got.Sizes) != 3 || got.Sizes[0] != 128 || got.Sizes[1] != 128 || got.Sizes[2] != 4 {
		t.Fatalf("query batch sizes = %#v", got.Sizes)
	}
	if len(got.LargeSizes) != 3 || got.LargeSizes[0] != 1 || got.LargeSizes[1] != 1 || got.LargeSizes[2] != 1 {
		t.Fatalf("query payload-size batches = %#v", got.LargeSizes)
	}
	if got.Timeout1 < 120000 || got.Timeout128 < got.Timeout1 || got.Timeout128 > 300000 || !got.FirstAborted || got.Querying {
		t.Fatalf("query cancellation/timeout state = %#v", got)
	}
	if len(got.ResultIDs) != 1 || got.ResultIDs[0] != "result-new" || len(got.Persisted) != 1 || got.Persisted[0][0] != "result-new" {
		t.Fatalf("superseded query overwrote current results: %#v", got)
	}
	if got.FalseBusyCount != 1 {
		t.Fatalf("superseded finally changed busy state: %#v", got)
	}
}

func TestDashboardQueryKeepsLastSuccessAndUnavailableProviderPreview(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	page := string(RenderDashboard(300))
	start := strings.Index(page, "  function mergeQueryResults(accounts, results, previousResults)")
	end := strings.Index(page, "  function queryBalances(refresh)")
	if start < 0 || end <= start {
		t.Fatal("cannot locate dashboard result merge helper")
	}
	script := `
var state = {previewAccounts:[{id:"stale",provider_key:"offline",digest:"offline-v1"}]};
function accountResultDigest(account) { return String(account && (account.__account_digest || account.digest) || ""); }
function attachResultAccountDigest(result, digest) { Object.defineProperty(result, "__account_digest", {value:String(digest),configurable:true}); return result; }
function resultMatchesAccount(result, account) {
  return Boolean(result && result.__account_digest) && result.__account_digest === accountResultDigest(account) &&
    String(result && result.auth_id || "") === String(account && account.id || "") &&
    String(result && result.provider_key || "") === String(account && account.provider_key || "");
}
function appendUnavailablePreviewResults(results, previousResults, liveAccounts, previewAccounts) {
  var merged = results.slice();
  previousResults.forEach(function (previous) {
    var live = liveAccounts.some(function (account) { return resultMatchesAccount(previous, account); });
    var preview = previewAccounts.filter(function (account) { return resultMatchesAccount(previous, account); })[0];
    if (!live && preview && !merged.some(function (candidate) { return resultMatchesAccount(candidate, preview); })) merged.push(previous);
  });
  return merged;
}
` + page[start:end] + `
var accounts = [{id:"a",provider_key:"live",digest:"a-v2"},{id:"b",provider_key:"live",digest:"b-v2"}];
var previous = [
  {auth_id:"a",provider_key:"live",quota_display:"上次成功",__account_digest:"a-v2"},
  {auth_id:"b",provider_key:"live",quota_display:"旧密钥的成功结果",__account_digest:"b-v1"},
  {auth_id:"stale",provider_key:"offline",quota_display:"离线来源的上次结果",__account_digest:"offline-v1"}
];
var current = [
  {auth_id:"a",provider_key:"live",error:"HTTP 503",failure:{retryable:true}},
  {auth_id:"b",provider_key:"live",error:"HTTP 401",failure:{retryable:false}}
];
var merged = mergeQueryResults(accounts, current, previous);
process.stdout.write(JSON.stringify(merged));
`
	output, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("execute dashboard result merge helper: %v\n%s", err, output)
	}
	var got struct {
		RetainedFailures int `json:"retainedFailures"`
		Results          []struct {
			AuthID       string `json:"auth_id"`
			QuotaDisplay string `json:"quota_display"`
			Error        string `json:"error"`
		} `json:"results"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode dashboard result merge: %v\n%s", err, output)
	}
	if got.RetainedFailures != 1 || len(got.Results) != 3 || got.Results[0].QuotaDisplay != "上次成功" || got.Results[0].Error != "" || got.Results[1].Error == "" || got.Results[1].QuotaDisplay != "" || got.Results[2].AuthID != "stale" {
		t.Fatalf("last-success/preview merge = %#v", got)
	}
}

func TestDashboardPartialProviderPreviewOnlyRetainsFailedSources(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	page := string(RenderDashboard(300))
	start := strings.Index(page, "  function canonicalProviderSource(value, category)")
	end := strings.Index(page, "  function providerCategoryRank(value)")
	if start < 0 || end <= start {
		t.Fatal("cannot locate dashboard provider-source helpers")
	}
	script := `
function providerCategoryLabel(value) {
  var labels = {"openai-compatibility":"OpenAI 兼容","claude-api-key":"Claude","xai-api-key":"xAI","codex-api-key":"Codex","gemini-api-key":"Gemini"};
  return labels[String(value || "")] || String(value || "OpenAI 兼容");
}
function snapshotDisplayProvider(provider) { return Object.assign({}, provider, {keys:[]}); }
function snapshotDisplayAccount(account) { return Object.assign({}, account); }
` + page[start:end] + `
var providers = [
  {name:"已删除但来源读取成功",category:"OpenAI 兼容",mappingKey:"openai-old",legacyMappingKey:"openai-old"},
  {name:"Claude 上次配置",category:"Claude",mappingKey:"claude-old",legacyMappingKey:"claude-old"},
  {name:"Gemini 上次配置",category:"Gemini",source:"gemini-api-key",mappingKey:"gemini-old",legacyMappingKey:"gemini-old"}
];
var accounts = [
  {id:"openai-account",provider_key:"openai-old"},
  {id:"claude-account",provider_key:"claude-old"},
  {id:"gemini-account",provider_key:"gemini-old"}
];
var preview = previewForFailedProviderSources(providers, accounts, ["claude-api-key"]);
process.stdout.write(JSON.stringify({
  provider_names:preview.providers.map(function (provider) { return provider.name; }),
  provider_sources:preview.providers.map(function (provider) { return provider.source; }),
  account_ids:preview.accounts.map(function (account) { return account.id; }),
  account_sources:preview.accounts.map(function (account) { return account.source; })
}));
`
	output, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("execute dashboard provider-source helpers: %v\n%s", err, output)
	}
	var got struct {
		ProviderNames   []string `json:"provider_names"`
		ProviderSources []string `json:"provider_sources"`
		AccountIDs      []string `json:"account_ids"`
		AccountSources  []string `json:"account_sources"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode dashboard provider-source result: %v\n%s", err, output)
	}
	if !reflect.DeepEqual(got.ProviderNames, []string{"Claude 上次配置"}) ||
		!reflect.DeepEqual(got.ProviderSources, []string{"claude-api-key"}) ||
		!reflect.DeepEqual(got.AccountIDs, []string{"claude-account"}) ||
		!reflect.DeepEqual(got.AccountSources, []string{"claude-api-key"}) {
		t.Fatalf("partial provider preview retained unrelated sources: %#v", got)
	}
}

func TestDashboardSaveIgnoresResponseAfterReconnectWithSameCredentials(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	page := string(RenderDashboard(300))
	cancelStart := strings.Index(page, "  function cancelLoadRequests()")
	cancelEnd := strings.Index(page, "  function showConnection(message)")
	saveStart := strings.Index(page, "  function validateTTL()")
	saveEnd := strings.Index(page, "  function loadData(options)")
	if cancelStart < 0 || cancelEnd <= cancelStart || saveStart < 0 || saveEnd <= saveStart {
		t.Fatal("cannot locate dashboard save request helpers")
	}
	script := `
var REQUEST_TIMEOUT = 30000;
var CONFIG_REQUEST_TIMEOUT = 10000;
var providerLabels = Object.create(null);
var state = {
  credentials:{apiBase:"https://cpa.example",managementKey:"same-secret"},
  providers:[],config:{cache_ttl_seconds:300,provider_mappings:{}},draftMappings:{},draftTTL:"600",
  draftRevision:1,dataReady:true,
  results:[{id:"new-load-result"}],view:"settings",dirty:true,needsQuery:false,
  saving:false,saveGeneration:0,saveController:null,loadGeneration:7,loadController:null,
  querying:false,queryGeneration:0,queryController:null
};
var resolveRequest;
var buttonStates = [];
var saveStates = [];
var toastCount = 0;
var renderCount = 0;
var persistCount = 0;
var requestSignal = null;
function byID() { return {}; }
function setButtonBusy(_button, busy) { buttonStates.push(Boolean(busy)); }
function setText(_node, value) { saveStates.push(String(value || "")); }
function toast() { toastCount += 1; }
function owns(value, key) { return Object.prototype.hasOwnProperty.call(value || {}, key); }
function stableMappings(value) { return JSON.stringify(value || {}); }
function normalizeConfig(value) { return {cache_ttl_seconds:Number(value.cache_ttl_seconds),provider_mappings:Object.assign({},value.provider_mappings || {})}; }
function resolvedProviderMapping() { return {key:"",type:""}; }
function mappedQueryType() { return ""; }
function refreshDirtyState() { return state.dirty; }
function waitForRuntimeConfig() { return Promise.resolve(state.config); }
function withConfigSaveLock(_credentials, _controller, callback) { return callback(function () {}); }
function apiFetch(_path, options) {
  requestSignal = options.signal;
  return new Promise(function (resolve) { resolveRequest = resolve; });
}
function clearResultSnapshot() {}
function persistResultSnapshot() { persistCount += 1; return Promise.resolve(true); }
function buildAccounts() { return []; }
function resultsMatchAccounts() { return false; }
function renderSettings() { renderCount += 1; }
function renderResults() { renderCount += 1; }
function queryBalances() { renderCount += 1; return Promise.resolve(); }
var window = {setTimeout:function () {}};
` + page[cancelStart:cancelEnd] + page[saveStart:saveEnd] + `
(async function () {
  var saving = saveSettings();
  var savingBeforeReconnect = state.saving;
  cancelSaveRequests();
  cancelLoadRequests();
  state.config = {cache_ttl_seconds:900,provider_mappings:{new_load:"claude"}};
  state.draftMappings = {new_load:"claude"};
  state.draftTTL = "900";
  state.dirty = false;
  resolveRequest({});
  var saveResult = await saving;
  process.stdout.write(JSON.stringify({
    saving_before_reconnect:savingBeforeReconnect,
    signal_aborted:requestSignal && requestSignal.aborted,
    save_result:saveResult,
    saving_after:state.saving,
    ttl_after:state.config.cache_ttl_seconds,
    mappings_after:state.config.provider_mappings,
    toast_count:toastCount,
    render_count:renderCount,
    persist_count:persistCount,
    saved_state_seen:saveStates.indexOf("已保存") !== -1,
    false_busy_count:buttonStates.filter(function (busy) { return !busy; }).length
  }));
})().catch(function (error) { process.stderr.write(String(error && error.stack || error)); process.exit(1); });
`
	output, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("execute dashboard save race helpers: %v\n%s", err, output)
	}
	var got struct {
		SavingBeforeReconnect bool              `json:"saving_before_reconnect"`
		SignalAborted         bool              `json:"signal_aborted"`
		SaveResult            bool              `json:"save_result"`
		SavingAfter           bool              `json:"saving_after"`
		TTLAfter              int               `json:"ttl_after"`
		MappingsAfter         map[string]string `json:"mappings_after"`
		ToastCount            int               `json:"toast_count"`
		RenderCount           int               `json:"render_count"`
		PersistCount          int               `json:"persist_count"`
		SavedStateSeen        bool              `json:"saved_state_seen"`
		FalseBusyCount        int               `json:"false_busy_count"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode dashboard save race result: %v\n%s", err, output)
	}
	if !got.SavingBeforeReconnect || !got.SignalAborted || got.SaveResult || got.SavingAfter {
		t.Fatalf("save request was not invalidated by reconnect: %#v", got)
	}
	if got.TTLAfter != 900 || got.MappingsAfter["new_load"] != "claude" || got.ToastCount != 0 || got.RenderCount != 0 || got.PersistCount != 0 || got.SavedStateSeen {
		t.Fatalf("stale save response overwrote the reloaded state: %#v", got)
	}
	if got.FalseBusyCount != 1 {
		t.Fatalf("stale save finally changed the new UI state: %#v", got)
	}
}

func TestDashboardWaitsForRuntimeConfigToMatch(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	page := string(RenderDashboard(300))
	start := strings.Index(page, "  function abortableDelay(milliseconds, signal)")
	end := strings.Index(page, "  function legacyNormalizeBaseForKey(value)")
	if start < 0 || end <= start {
		t.Fatal("cannot locate runtime-config wait helpers")
	}
	script := `
var CONFIG_REQUEST_TIMEOUT = 10000;
var CONFIG_STATE_WAIT_TIMEOUT = 15000;
var RUNTIME_CONFIG_PATH = "/balance-query/config-state";
var providerLabels = {newapi:"New API",deepseek:"DeepSeek"};
var calls = 0;
var replies = [
  {cache_ttl_seconds:300,provider_mappings:{one:"newapi"}},
  {cache_ttl_seconds:600,provider_mappings:{one:"newapi"}},
  {cache_ttl_seconds:600,provider_mappings:{two:"deepseek",one:"newapi"}}
];
var window = {setTimeout:function (fn) { return setTimeout(fn, 0); },clearTimeout:clearTimeout};
function normalizeConfig(raw) { return {cache_ttl_seconds:Number(raw.cache_ttl_seconds),provider_mappings:Object.assign({},raw.provider_mappings || {})}; }
function stableMappings(value) { return JSON.stringify(Object.keys(value || {}).sort().map(function (key) { return [key,value[key]]; })); }
function apiFetch() { var reply = replies[Math.min(calls, replies.length - 1)]; calls += 1; return Promise.resolve(reply); }
function optionalApiFetch() { return Promise.resolve({}); }
` + page[start:end] + `
(async function () {
  var expected = {cache_ttl_seconds:600,provider_mappings:{one:"newapi",two:"deepseek"}};
  var resolved = await waitForRuntimeConfig(expected, new AbortController(), {});
  process.stdout.write(JSON.stringify({calls:calls,resolved:resolved,equal:configsEqual(resolved, expected)}));
})().catch(function (error) { process.stderr.write(String(error && error.stack || error)); process.exit(1); });
`
	output, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("execute runtime-config wait helpers: %v\n%s", err, output)
	}
	var got struct {
		Calls    int  `json:"calls"`
		Equal    bool `json:"equal"`
		Resolved struct {
			TTL int `json:"cache_ttl_seconds"`
		} `json:"resolved"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode runtime-config wait result: %v\n%s", err, output)
	}
	if got.Calls != 3 || !got.Equal || got.Resolved.TTL != 600 {
		t.Fatalf("runtime config was accepted before both fields matched: %#v", got)
	}
}

func TestDashboardLoadReconcilesMovingPersistedAndRuntimeConfig(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	page := string(RenderDashboard(300))
	start := strings.Index(page, "  function abortableDelay(milliseconds, signal)")
	end := strings.Index(page, "  function legacyNormalizeBaseForKey(value)")
	if start < 0 || end <= start {
		t.Fatal("cannot locate synchronized config helpers")
	}
	script := `
var CONFIG_REQUEST_TIMEOUT = 10000;
var CONFIG_STATE_WAIT_TIMEOUT = 15000;
var RUNTIME_CONFIG_PATH = "/balance-query/config-state";
var providerLabels = {newapi:"New API",deepseek:"DeepSeek"};
var hostCalls = 0;
var runtimeCalls = 0;
var configA = {cache_ttl_seconds:300,provider_mappings:{one:"newapi"}};
var configB = {cache_ttl_seconds:600,provider_mappings:{one:"deepseek"}};
var window = {setTimeout:function (fn) { return setTimeout(fn, 0); },clearTimeout:clearTimeout};
function normalizeConfig(raw) { return {cache_ttl_seconds:Number(raw.cache_ttl_seconds),provider_mappings:Object.assign({},raw.provider_mappings || {})}; }
function stableMappings(value) { return JSON.stringify(Object.keys(value || {}).sort().map(function (key) { return [key,value[key]]; })); }
function apiFetch(path) {
  if (path === "/plugins/balance-query/config") {
    hostCalls += 1;
    return Promise.resolve(hostCalls === 1 ? configA : configB);
  }
  runtimeCalls += 1;
  return Promise.resolve(configB);
}
function optionalApiFetch() { return Promise.resolve({}); }
` + page[start:end] + `
(async function () {
  var resolved = await waitForSynchronizedConfig(new AbortController(), {});
  process.stdout.write(JSON.stringify({host_calls:hostCalls,runtime_calls:runtimeCalls,resolved:resolved}));
})().catch(function (error) { process.stderr.write(String(error && error.stack || error)); process.exit(1); });
`
	output, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("execute synchronized config helpers: %v\n%s", err, output)
	}
	var got struct {
		HostCalls    int `json:"host_calls"`
		RuntimeCalls int `json:"runtime_calls"`
		Resolved     struct {
			TTL      int               `json:"cache_ttl_seconds"`
			Mappings map[string]string `json:"provider_mappings"`
		} `json:"resolved"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode synchronized config result: %v\n%s", err, output)
	}
	if got.HostCalls != 2 || got.RuntimeCalls != 2 || got.Resolved.TTL != 600 || got.Resolved.Mappings["one"] != "deepseek" {
		t.Fatalf("moving host/runtime target did not converge: %#v", got)
	}
}

func TestDashboardSaveWaitsForUnrelatedRuntimeChanges(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	page := string(RenderDashboard(300))
	start := strings.Index(page, "  function abortableDelay(milliseconds, signal)")
	end := strings.Index(page, "  function legacyNormalizeBaseForKey(value)")
	if start < 0 || end <= start {
		t.Fatal("cannot locate runtime config confirmation helpers")
	}
	script := `
var CONFIG_REQUEST_TIMEOUT = 10000;
var CONFIG_STATE_WAIT_TIMEOUT = 15000;
var RUNTIME_CONFIG_PATH = "/balance-query/config-state";
var providerLabels = {newapi:"New API",deepseek:"DeepSeek"};
var runtimeCalls = 0;
var persisted = {cache_ttl_seconds:300,provider_mappings:{a:"newapi",b:"deepseek"}};
var runtimeReplies = [
  {cache_ttl_seconds:300,provider_mappings:{a:"newapi"}},
  {cache_ttl_seconds:300,provider_mappings:{a:"newapi",b:"deepseek"}}
];
var window = {setTimeout:function (fn) { return setTimeout(fn, 0); },clearTimeout:clearTimeout};
function normalizeConfig(raw) { return {cache_ttl_seconds:Number(raw.cache_ttl_seconds),provider_mappings:Object.assign({},raw.provider_mappings || {})}; }
function stableMappings(value) { return JSON.stringify(Object.keys(value || {}).sort().map(function (key) { return [key,value[key]]; })); }
function configMatchesChanges(current, expected) { return current.provider_mappings.a === expected.provider_mappings.a; }
function settingsConflict(message) { var error = new Error(message); error.status = 409; return error; }
function apiFetch(path) {
  if (path === RUNTIME_CONFIG_PATH) {
    var reply = runtimeReplies[Math.min(runtimeCalls, runtimeReplies.length - 1)];
    runtimeCalls += 1;
    return Promise.resolve(reply);
  }
  return Promise.resolve(persisted);
}
function optionalApiFetch() { return Promise.resolve({}); }
` + page[start:end] + `
(async function () {
  var expected = {cache_ttl_seconds:300,provider_mappings:{a:"newapi"}};
  var resolved = await waitForRuntimeConfig(expected, new AbortController(), {}, {ttlChanged:false,providers:[{mappingKey:"a",legacyMappingKey:"a"}]});
  process.stdout.write(JSON.stringify({runtime_calls:runtimeCalls,resolved:resolved}));
})().catch(function (error) { process.stderr.write(String(error && error.stack || error)); process.exit(1); });
`
	output, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("execute runtime config delta confirmation: %v\n%s", err, output)
	}
	var got struct {
		RuntimeCalls int `json:"runtime_calls"`
		Resolved     struct {
			Mappings map[string]string `json:"provider_mappings"`
		} `json:"resolved"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode runtime config delta confirmation: %v\n%s", err, output)
	}
	if got.RuntimeCalls != 2 || got.Resolved.Mappings["b"] != "deepseek" {
		t.Fatalf("save was confirmed before unrelated runtime config converged: %#v", got)
	}
}

func TestDashboardProxyConfigFailureKeepsPreviewUsableAndSchedulesRecovery(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	page := string(RenderDashboard(300))
	start := strings.Index(page, "  function loadData(options)")
	end := strings.Index(page, `  document.querySelectorAll(".segment")`)
	if start < 0 || end <= start {
		t.Fatal("cannot locate dashboard load helper")
	}
	script := `
var CONFIG_REQUEST_TIMEOUT = 10000;
var RUNTIME_CONFIG_PATH = "/balance-query/config-state";
var state = {
  credentials:{apiBase:"https://cpa.example",managementKey:"secret"},
  providers:[],previewProviders:[],previewAccounts:[],
  config:{cache_ttl_seconds:300,provider_mappings:{old:"newapi"}},
  draftMappings:{},draftTTL:"300",draftRevision:0,results:[],
  loadController:null,loadGeneration:0,snapshotGeneration:0,dataReady:true,
  runtimeSyncPending:false,configRecoveryTimer:null,configRecoveryController:null,configRecoveryAttempt:0
};
var queryCalls = 0;
var connectionCalls = 0;
var recoveryCalls = 0;
var messages = [];
function cancelConfigRecovery() {}
function scheduleConfigRecovery(message) { recoveryCalls += 1; state.runtimeSyncPending = true; messages.push(String(message || "")); }
function cancelSaveRequests() {}
function cancelQueryRequests() {}
function displayProviders() { return state.providers.slice(); }
function displayAccounts() { return state.previewAccounts.slice(); }
function snapshotDisplayAccount(account) { return Object.assign({}, account); }
function snapshotDisplayProvider(provider) { return Object.assign({}, provider, {keys:[]}); }
function setView() {}
function setDataReady(value) { state.dataReady = Boolean(value); }
function showApp() {}
function showAppRecovering(message) { messages.push("已连接 CPA · 插件配置同步中"); messages.push(String(message || "")); }
function renderResults() {}
function renderSettings() {}
function updateSummary() {}
function refreshDirtyState() { state.dirty = false; }
function previewForFailedProviderSources() { return {providers:[],accounts:[]}; }
function setText(_node, value) { messages.push(String(value || "")); }
function byID() { return {}; }
function showSkeletons() {}
function readResultSnapshotPreview() {
  return new Promise(function (resolve) {
    setTimeout(function () { resolve({
      providers:[{name:"上次的服务",mappingKey:"old",legacyMappingKey:"old",disabled:false}],
      accounts:[{id:"account",provider_key:"old"}],
      config:{cache_ttl_seconds:300,provider_mappings:{old:"newapi"}},
      results:[{auth_id:"account",provider_key:"old",quota_display:"上次成功"}]
    }); }, 5);
  });
}

function tolerantConfigFetch() { return Promise.resolve({data:{},error:null}); }
function optionalApiFetch(path) {
  if (path === "/proxy-url") {
    var error = new Error("代理配置接口暂时不可用");
    error.status = 503;
    return Promise.reject(error);
  }
  return Promise.resolve({});
}
function apiFetch() { return Promise.resolve({cache_ttl_seconds:300,provider_mappings:{old:"newapi"}}); }
function queryBalances() { queryCalls += 1; return Promise.resolve(); }
function showConnection() { connectionCalls += 1; }
function toast(message) { messages.push(String(message || "")); }
` + page[start:end] + `
(async function () {
  await loadData();
  process.stdout.write(JSON.stringify({
    query_calls:queryCalls,connection_calls:connectionCalls,result_count:state.results.length,
    data_ready:state.dataReady,recovery_calls:recoveryCalls,messages:messages
  }));
})().catch(function (error) { process.stderr.write(String(error && error.stack || error)); process.exit(1); });
`
	output, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("execute proxy failure load helper: %v\n%s", err, output)
	}
	var got struct {
		QueryCalls      int      `json:"query_calls"`
		ConnectionCalls int      `json:"connection_calls"`
		ResultCount     int      `json:"result_count"`
		DataReady       bool     `json:"data_ready"`
		RecoveryCalls   int      `json:"recovery_calls"`
		Messages        []string `json:"messages"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode proxy failure result: %v\n%s", err, output)
	}
	joined := strings.Join(got.Messages, " ")
	if got.QueryCalls != 0 || got.ConnectionCalls != 0 || got.ResultCount != 1 || !got.DataReady || got.RecoveryCalls != 1 || !strings.Contains(joined, "已连接 CPA · 插件配置同步中") {
		t.Fatalf("proxy failure did not preserve a usable preview and schedule recovery: %#v", got)
	}
}

func TestDashboardConfigRecoveryRetriesAndReloadsAutomatically(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	page := string(RenderDashboard(300))
	start := strings.Index(page, "  function cancelConfigRecovery(resetAttempt)")
	end := strings.Index(page, "  function showConnection(message)")
	if start < 0 || end <= start {
		t.Fatal("cannot locate dashboard config recovery helpers")
	}
	script := `
var queued = [];
var timerID = 0;
var window = {
  setTimeout:function (fn) { queued.push(fn); timerID += 1; return timerID; },
  clearTimeout:function () {}
};
var state = {
  credentials:{apiBase:"https://cpa.example",managementKey:"secret"},loadGeneration:3,
  configRecoveryTimer:null,configRecoveryController:null,configRecoveryAttempt:0,
  runtimeSyncPending:false,saving:false,dirty:false,querying:false,dataReady:true
};
var waitCalls = 0;
var loadCalls = 0;
var statuses = [];
function showAppRecovering(message) { statuses.push(String(message || "")); }
function showApp() {}
function setDataReady() {}
function renderSettings() {}
function refreshDirtyState() {}
function toast(message) { statuses.push(String(message || "")); }
function loadData(options) { loadCalls += 1; return Promise.resolve(Boolean(options && options.fromRecovery)); }
function waitForSynchronizedConfig() {
  waitCalls += 1;
  if (waitCalls === 1) { var error = new Error("尚未同步"); error.runtimePending = true; return Promise.reject(error); }
  return Promise.resolve({cache_ttl_seconds:300,provider_mappings:{}});
}
` + page[start:end] + `
(async function () {
  scheduleConfigRecovery("正在恢复");
  for (var step = 0; step < 6 && !loadCalls; step++) {
    var fn = queued.shift();
    if (fn) fn();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
  }
  process.stdout.write(JSON.stringify({wait_calls:waitCalls,load_calls:loadCalls,pending:state.runtimeSyncPending,statuses:statuses}));
})().catch(function (error) { process.stderr.write(String(error && error.stack || error)); process.exit(1); });
`
	output, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("execute dashboard config recovery: %v\n%s", err, output)
	}
	var got struct {
		WaitCalls int      `json:"wait_calls"`
		LoadCalls int      `json:"load_calls"`
		Pending   bool     `json:"pending"`
		Statuses  []string `json:"statuses"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode dashboard config recovery: %v\n%s", err, output)
	}
	if got.WaitCalls != 2 || got.LoadCalls != 1 || got.Pending || !strings.Contains(strings.Join(got.Statuses, " "), "自动恢复同步") {
		t.Fatalf("config recovery did not retry and reload automatically: %#v", got)
	}
}

func TestDashboardKeepsSettingsEditableWhileRuntimeSynchronizes(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	page := string(RenderDashboard(300))
	start := strings.Index(page, "  function setDataReady(ready)")
	end := strings.Index(page, "  function setButtonBusy(button, busy, label)")
	if start < 0 || end <= start {
		t.Fatal("cannot locate dashboard data-ready helper")
	}
	script := `
var nodes = {"tab-settings":{},"refresh-button":{},"save-button":{},"ttl-input":{}};
var state = {dataReady:false,runtimeSyncPending:true,querying:false,saving:false,dirty:true};
function byID(id) { return nodes[id]; }
` + page[start:end] + `
setDataReady(true);
process.stdout.write(JSON.stringify({tab:nodes["tab-settings"].disabled,refresh:nodes["refresh-button"].disabled,save:nodes["save-button"].disabled,ttl:nodes["ttl-input"].disabled}));
`
	output, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("execute dashboard pending controls: %v\n%s", err, output)
	}
	var got struct {
		Tab     bool `json:"tab"`
		Refresh bool `json:"refresh"`
		Save    bool `json:"save"`
		TTL     bool `json:"ttl"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode dashboard pending controls: %v\n%s", err, output)
	}
	if got.Tab || !got.Refresh || got.Save || got.TTL {
		t.Fatalf("runtime synchronization controls = %#v, want settings and retry-save usable while balance refresh stays disabled", got)
	}
}

func TestDashboardRuntimePendingCanResubmitUnchangedSettings(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	page := string(RenderDashboard(300))
	start := strings.Index(page, "  function validateTTL()")
	end := strings.Index(page, "  function loadData(options)")
	if start < 0 || end <= start {
		t.Fatal("cannot locate dashboard save helpers")
	}
	script := `
var REQUEST_TIMEOUT = 30000;
var CONFIG_REQUEST_TIMEOUT = 10000;
var providerLabels = {newapi:"New API"};
var state = {
  credentials:{apiBase:"https://cpa.example",managementKey:"secret"},
  providers:[{name:"服务 A",mappingKey:"a",legacyMappingKey:"legacy-a"}],
  config:{cache_ttl_seconds:300,provider_mappings:{a:"newapi"}},
  draftMappings:{a:"newapi"},draftTTL:"300",draftRevision:0,
  results:[],view:"settings",dirty:false,needsQuery:false,dataReady:true,
  runtimeSyncPending:true,saving:false,saveGeneration:0,saveController:null,loadGeneration:2,
  querying:false,queryGeneration:0,queryController:null
};
var patched = null;
var recoveryCancels = 0;
function byID() { return {}; }
function setButtonBusy() {}
function setText() {}
function toast() {}
function owns(value, key) { return Object.prototype.hasOwnProperty.call(value || {}, key); }
function stableMappings(value) { return JSON.stringify(Object.keys(value || {}).sort().map(function (key) { return [key,value[key]]; })); }
function normalizeConfig(value) { return {cache_ttl_seconds:Number(value.cache_ttl_seconds),provider_mappings:Object.assign({},value.provider_mappings || {})}; }
function mappedQueryType(provider, mappings) { return mappings[provider.mappingKey] || mappings[provider.legacyMappingKey] || ""; }
function resolvedProviderMapping(provider, mappings) {
  if (mappings[provider.mappingKey]) return {key:provider.mappingKey,type:mappings[provider.mappingKey]};
  if (mappings[provider.legacyMappingKey]) return {key:provider.legacyMappingKey,type:mappings[provider.legacyMappingKey]};
  return {key:provider.mappingKey,type:""};
}
function refreshDirtyState() { state.dirty = false; return false; }
function cancelConfigRecovery() { recoveryCancels += 1; }
function scheduleConfigRecovery() {}
function apiFetch(_path, options) {
  if (options && options.method === "PATCH") { patched = JSON.parse(options.body); return Promise.resolve({}); }
  return Promise.resolve({cache_ttl_seconds:300,provider_mappings:{a:"newapi"}});
}
function waitForRuntimeConfig(expected) { return Promise.resolve(normalizeConfig(expected)); }
function withConfigSaveLock(_credentials, _controller, callback) { return callback(function () {}); }
function showApp() {}
function setDataReady(value) { state.dataReady = Boolean(value); }
function cancelQueryRequests() {}
function clearResultSnapshot() {}
function persistResultSnapshot() { return Promise.resolve(true); }
function buildAccounts() { return []; }
function resultsMatchAccounts() { return false; }
function renderSettings() {}
function renderResults() {}
function queryBalances() { return Promise.resolve(); }
var window = {setTimeout:function () {}};
` + page[start:end] + `
(async function () {
  var saved = await saveSettings();
  process.stdout.write(JSON.stringify({saved:saved,patched:patched,recovery_cancels:recoveryCancels,pending:state.runtimeSyncPending}));
})().catch(function (error) { process.stderr.write(String(error && error.stack || error)); process.exit(1); });
`
	output, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("execute dashboard runtime retry save: %v\n%s", err, output)
	}
	var got struct {
		Saved           bool `json:"saved"`
		RecoveryCancels int  `json:"recovery_cancels"`
		Pending         bool `json:"pending"`
		Patched         struct {
			CacheTTLSeconds  int               `json:"cache_ttl_seconds"`
			ProviderMappings map[string]string `json:"provider_mappings"`
		} `json:"patched"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode dashboard runtime retry save: %v\n%s", err, output)
	}
	if !got.Saved || got.RecoveryCancels != 2 || got.Pending || got.Patched.CacheTTLSeconds != 300 || got.Patched.ProviderMappings["a"] != "newapi" {
		t.Fatalf("runtime retry save did not re-submit the complete config: %#v", got)
	}
}

func TestDashboardFallbackConfigSaveLockSerializesTabs(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	page := string(RenderDashboard(300))
	storageStart := strings.Index(page, "  function sameOriginLocalStorage()")
	storageEnd := strings.Index(page, "  function discardStoredResultSnapshot")
	delayStart := strings.Index(page, "  function abortableDelay(milliseconds, signal)")
	delayEnd := strings.Index(page, "  function configsEqual(left, right)")
	lockStart := strings.Index(page, "  function configSaveLockError(message, lost)")
	lockEnd := strings.Index(page, "  function validateTTL()")
	if storageStart < 0 || storageEnd <= storageStart || delayStart < 0 || delayEnd <= delayStart || lockStart < 0 || lockEnd <= lockStart {
		t.Fatal("cannot locate dashboard fallback lock helpers")
	}
	script := `
var memory = Object.create(null);
var REQUEST_TIMEOUT = 30000;
var CONFIG_REQUEST_TIMEOUT = 10000;
var CONFIG_STATE_WAIT_TIMEOUT = 15000;
var storage = {
  get length() { return Object.keys(memory).length; },
  key:function (index) { return Object.keys(memory)[index] || null; },
  getItem:function (key) { return Object.prototype.hasOwnProperty.call(memory, key) ? memory[key] : null; },
  setItem:function (key, value) { memory[key] = String(value); },
  removeItem:function (key) { delete memory[key]; }
};
var window = {
  localStorage:storage,location:{origin:"https://cpa.example"},
  setTimeout:setTimeout,clearTimeout:clearTimeout,setInterval:setInterval,clearInterval:clearInterval
};
window.parent = window;
var navigator = {};
function tinyHash(value) { return String(value).replace(/[^a-z0-9]/gi, ""); }
` + page[storageStart:storageEnd] + page[delayStart:delayEnd] + page[lockStart:lockEnd] + `
(async function () {
  var order = [];
  var credentials = {apiBase:"https://cpa.example"};
  var first = withConfigSaveLock(credentials, new AbortController(), async function () {
    order.push("first-start");
    await new Promise(function (resolve) { setTimeout(resolve, 25); });
    order.push("first-end");
  });
  var second = withConfigSaveLock(credentials, new AbortController(), async function () {
    order.push("second-start");
    order.push("second-end");
  });
  await Promise.all([first, second]);
  process.stdout.write(JSON.stringify({order:order,remaining:storage.length}));
})().catch(function (error) { process.stderr.write(String(error && error.stack || error)); process.exit(1); });
`
	output, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("execute fallback config lock: %v\n%s", err, output)
	}
	var got struct {
		Order     []string `json:"order"`
		Remaining int      `json:"remaining"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode fallback config lock result: %v\n%s", err, output)
	}
	want := []string{"first-start", "first-end", "second-start", "second-end"}
	if !reflect.DeepEqual(got.Order, want) || got.Remaining != 0 {
		t.Fatalf("fallback config lock = %#v, want order %#v and cleanup", got, want)
	}
}

func TestDashboardFallbackConfigSaveLockFailsClosedWithoutStorage(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	page := string(RenderDashboard(300))
	storageStart := strings.Index(page, "  function sameOriginLocalStorage()")
	storageEnd := strings.Index(page, "  function discardStoredResultSnapshot")
	delayStart := strings.Index(page, "  function abortableDelay(milliseconds, signal)")
	delayEnd := strings.Index(page, "  function configsEqual(left, right)")
	lockStart := strings.Index(page, "  function configSaveLockError(message, lost)")
	lockEnd := strings.Index(page, "  function validateTTL()")
	if storageStart < 0 || storageEnd <= storageStart || delayStart < 0 || delayEnd <= delayStart || lockStart < 0 || lockEnd <= lockStart {
		t.Fatal("cannot locate dashboard fallback lock helpers")
	}
	script := `
var REQUEST_TIMEOUT = 30000;
var CONFIG_REQUEST_TIMEOUT = 10000;
var CONFIG_STATE_WAIT_TIMEOUT = 15000;
var window = {
  localStorage:null,location:{origin:"https://cpa.example"},
  setTimeout:setTimeout,clearTimeout:clearTimeout,setInterval:setInterval,clearInterval:clearInterval
};
window.parent = window;
var navigator = {};
function tinyHash(value) { return String(value).replace(/[^a-z0-9]/gi, ""); }
` + page[storageStart:storageEnd] + page[delayStart:delayEnd] + page[lockStart:lockEnd] + `
async function attempt(storage) {
  window.localStorage = storage;
  var callbackCalls = 0;
  try {
    await withConfigSaveLock({apiBase:"https://cpa.example"}, new AbortController(), function () {
      callbackCalls += 1;
    });
    return {rejected:false,callback_calls:callbackCalls,message:"",unavailable:false};
  } catch (error) {
    return {rejected:true,callback_calls:callbackCalls,message:String(error && error.message || ""),unavailable:Boolean(error && error.lockUnavailable)};
  }
}
(async function () {
  var noStorage = await attempt(null);
  var deniedStorage = await attempt({
    get length() { return 0; },key:function () { return null; },getItem:function () { return null; },
    setItem:function () { throw new Error("storage denied"); },removeItem:function () {}
  });
  process.stdout.write(JSON.stringify({no_storage:noStorage,denied_storage:deniedStorage}));
})().catch(function (error) { process.stderr.write(String(error && error.stack || error)); process.exit(1); });
`
	output, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("execute fallback lock fail-closed helpers: %v\n%s", err, output)
	}
	var got struct {
		NoStorage struct {
			Rejected      bool   `json:"rejected"`
			CallbackCalls int    `json:"callback_calls"`
			Message       string `json:"message"`
			Unavailable   bool   `json:"unavailable"`
		} `json:"no_storage"`
		DeniedStorage struct {
			Rejected      bool   `json:"rejected"`
			CallbackCalls int    `json:"callback_calls"`
			Message       string `json:"message"`
			Unavailable   bool   `json:"unavailable"`
		} `json:"denied_storage"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode fallback lock fail-closed result: %v\n%s", err, output)
	}
	for name, result := range map[string]struct {
		Rejected      bool
		CallbackCalls int
		Message       string
		Unavailable   bool
	}{
		"no storage":     {got.NoStorage.Rejected, got.NoStorage.CallbackCalls, got.NoStorage.Message, got.NoStorage.Unavailable},
		"denied storage": {got.DeniedStorage.Rejected, got.DeniedStorage.CallbackCalls, got.DeniedStorage.Message, got.DeniedStorage.Unavailable},
	} {
		if !result.Rejected || result.CallbackCalls != 0 || !result.Unavailable || !strings.Contains(result.Message, "未写入") {
			t.Fatalf("%s fallback lock did not fail closed: %#v", name, result)
		}
	}
}

func TestDashboardFallbackConfigSaveLockGuardRejectsExpiredOwnership(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	page := string(RenderDashboard(300))
	storageStart := strings.Index(page, "  function sameOriginLocalStorage()")
	storageEnd := strings.Index(page, "  function discardStoredResultSnapshot")
	delayStart := strings.Index(page, "  function abortableDelay(milliseconds, signal)")
	delayEnd := strings.Index(page, "  function configsEqual(left, right)")
	lockStart := strings.Index(page, "  function configSaveLockError(message, lost)")
	lockEnd := strings.Index(page, "  function validateTTL()")
	if storageStart < 0 || storageEnd <= storageStart || delayStart < 0 || delayEnd <= delayStart || lockStart < 0 || lockEnd <= lockStart {
		t.Fatal("cannot locate dashboard fallback lock helpers")
	}
	script := `
var REQUEST_TIMEOUT = 30000;
var CONFIG_REQUEST_TIMEOUT = 10000;
var CONFIG_STATE_WAIT_TIMEOUT = 15000;
var now = 1000;
Date.now = function () { return now; };
var memory = Object.create(null);
var storage = {
  get length() { return Object.keys(memory).length; },
  key:function (index) { return Object.keys(memory)[index] || null; },
  getItem:function (key) { return Object.prototype.hasOwnProperty.call(memory, key) ? memory[key] : null; },
  setItem:function (key, value) { memory[key] = String(value); },
  removeItem:function (key) { delete memory[key]; }
};
var window = {
  localStorage:storage,location:{origin:"https://cpa.example"},
  setTimeout:setTimeout,clearTimeout:clearTimeout,setInterval:setInterval,clearInterval:clearInterval
};
window.parent = window;
var navigator = {};
function tinyHash(value) { return String(value).replace(/[^a-z0-9]/gi, ""); }
` + page[storageStart:storageEnd] + page[delayStart:delayEnd] + page[lockStart:lockEnd] + `
(async function () {
  var patchCalls = 0;
  var leaseDuration = 0;
  var caught = null;
  try {
    await withConfigSaveLock({apiBase:"https://cpa.example"}, new AbortController(), function (assertOwnership) {
      var key = Object.keys(memory)[0];
      var record = JSON.parse(memory[key]);
      leaseDuration = Number(record.expires_at) - now;
      now = Number(record.expires_at) + 1;
      assertOwnership();
      patchCalls += 1;
    });
  } catch (error) {
    caught = error;
  }
  process.stdout.write(JSON.stringify({
    patch_calls:patchCalls,lease_duration:leaseDuration,remaining:storage.length,
    rejected:Boolean(caught),lost:Boolean(caught && caught.lockLost),message:String(caught && caught.message || "")
  }));
})().catch(function (error) { process.stderr.write(String(error && error.stack || error)); process.exit(1); });
`
	output, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("execute fallback lock ownership guard: %v\n%s", err, output)
	}
	var got struct {
		PatchCalls    int    `json:"patch_calls"`
		LeaseDuration int    `json:"lease_duration"`
		Remaining     int    `json:"remaining"`
		Rejected      bool   `json:"rejected"`
		Lost          bool   `json:"lost"`
		Message       string `json:"message"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode fallback lock ownership result: %v\n%s", err, output)
	}
	boundedSaveDuration := 30000 + 10000 + 15000
	if got.PatchCalls != 0 || got.LeaseDuration <= boundedSaveDuration || got.Remaining != 0 || !got.Rejected || !got.Lost || !strings.Contains(got.Message, "未写入") {
		t.Fatalf("expired fallback ownership was not rejected before write: %#v", got)
	}
}

func TestDashboardSavePreservesEditsMadeWhileRequestIsInFlight(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	page := string(RenderDashboard(300))
	start := strings.Index(page, "  function validateTTL()")
	end := strings.Index(page, "  function loadData(options)")
	if start < 0 || end <= start {
		t.Fatal("cannot locate dashboard save helpers")
	}
	script := `
var REQUEST_TIMEOUT = 30000;
var CONFIG_REQUEST_TIMEOUT = 10000;
var providerLabels = {newapi:"New API",deepseek:"DeepSeek"};
var state = {
  credentials:{apiBase:"https://cpa.example",managementKey:"secret"},
  providers:[
    {name:"服务 A",mappingKey:"a",legacyMappingKey:"legacy-a"},
    {name:"服务 B",mappingKey:"b",legacyMappingKey:"legacy-b"}
  ],
  config:{cache_ttl_seconds:300,provider_mappings:{a:"newapi"}},
  draftMappings:{a:"deepseek"},draftTTL:"600",draftRevision:1,
  results:[],view:"settings",dirty:true,needsQuery:false,dataReady:true,
  saving:false,saveGeneration:0,saveController:null,loadGeneration:4,
  querying:false,queryGeneration:0,queryController:null
};
var patchResolve;
var patched = null;
var saveStates = [];
var toasts = [];
function byID() { return {}; }
function setButtonBusy() {}
function setText(_node, value) { saveStates.push(String(value || "")); }
function toast(message) { toasts.push(message); }
function owns(value, key) { return Object.prototype.hasOwnProperty.call(value || {}, key); }
function stableMappings(value) { return JSON.stringify(Object.keys(value || {}).sort().map(function (key) { return [key,value[key]]; })); }
function normalizeConfig(value) { return {cache_ttl_seconds:Number(value.cache_ttl_seconds),provider_mappings:Object.assign({},value.provider_mappings || {})}; }
function mappedQueryType(provider, mappings) { return mappings[provider.mappingKey] || mappings[provider.legacyMappingKey] || ""; }
function resolvedProviderMapping(provider, mappings) {
  if (mappings[provider.mappingKey]) return {key:provider.mappingKey,type:mappings[provider.mappingKey]};
  if (mappings[provider.legacyMappingKey]) return {key:provider.legacyMappingKey,type:mappings[provider.legacyMappingKey]};
  return {key:provider.mappingKey,type:""};
}
function refreshDirtyState() {
  state.dirty = String(state.draftTTL) !== String(state.config.cache_ttl_seconds) || stableMappings(state.draftMappings) !== stableMappings(state.config.provider_mappings);
  return state.dirty;
}
function apiFetch(_path, options) {
  if (!options.method) return Promise.resolve({cache_ttl_seconds:300,provider_mappings:{a:"newapi",external:"claude"}});
  patched = JSON.parse(options.body);
  return new Promise(function (resolve) { patchResolve = resolve; });
}
function waitForRuntimeConfig(expected) { return Promise.resolve(normalizeConfig(expected)); }
function withConfigSaveLock(_credentials, _controller, callback) { return callback(function () {}); }
function cancelConfigRecovery() {}
function showApp() {}
function setDataReady(value) { state.dataReady = Boolean(value); }
function cancelQueryRequests() { state.querying = false; state.queryGeneration += 1; }
function clearResultSnapshot() {}
function persistResultSnapshot() { return Promise.resolve(true); }
function buildAccounts() { return []; }
function resultsMatchAccounts() { return false; }
function renderSettings() {}
function renderResults() {}
function queryBalances() { throw new Error("query must not start while settings view is active"); }
var window = {setTimeout:function () {}};
` + page[start:end] + `
(async function () {
  var saving = saveSettings();
  while (!patchResolve) await Promise.resolve();
  state.draftMappings.a = "newapi";
  state.draftMappings.b = "newapi";
  state.draftTTL = "300";
  state.draftRevision += 1;
  patchResolve({});
  var saved = await saving;
  process.stdout.write(JSON.stringify({
    saved:saved,patched:patched,config:state.config,draft:state.draftMappings,draft_ttl:state.draftTTL,
    dirty:state.dirty,saving:state.saving,save_states:saveStates,toasts:toasts
  }));
})().catch(function (error) { process.stderr.write(String(error && error.stack || error)); process.exit(1); });
`
	output, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("execute in-flight save helpers: %v\n%s", err, output)
	}
	var got struct {
		Saved   bool           `json:"saved"`
		Dirty   bool           `json:"dirty"`
		Saving  bool           `json:"saving"`
		Patched map[string]any `json:"patched"`
		Config  struct {
			TTL      int               `json:"cache_ttl_seconds"`
			Mappings map[string]string `json:"provider_mappings"`
		} `json:"config"`
		Draft      map[string]string `json:"draft"`
		DraftTTL   string            `json:"draft_ttl"`
		SaveStates []string          `json:"save_states"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode in-flight save result: %v\n%s", err, output)
	}
	if !got.Saved || !got.Dirty || got.Saving || got.Config.TTL != 600 || got.DraftTTL != "300" || got.Config.Mappings["a"] != "deepseek" || got.Draft["a"] != "newapi" || got.Draft["b"] != "newapi" {
		t.Fatalf("save response discarded a newer draft: %#v", got)
	}
	patchedMappings, ok := got.Patched["provider_mappings"].(map[string]any)
	if !ok || patchedMappings["a"] != "deepseek" || patchedMappings["external"] != "claude" {
		t.Fatalf("submitted config = %#v, want service A update plus the latest unrelated mapping", got.Patched)
	}
	if len(got.SaveStates) == 0 || !strings.Contains(got.SaveStates[len(got.SaveStates)-1], "仍有未保存") {
		t.Fatalf("save state did not report the remaining draft: %#v", got.SaveStates)
	}
}

func TestDashboardSaveReportsPersistedWhenRuntimeAuthCheckFails(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	page := string(RenderDashboard(300))
	start := strings.Index(page, "  function validateTTL()")
	end := strings.Index(page, "  function loadData(options)")
	if start < 0 || end <= start {
		t.Fatal("cannot locate dashboard save helpers")
	}
	script := `
var REQUEST_TIMEOUT = 30000;
var CONFIG_REQUEST_TIMEOUT = 10000;
var providerLabels = Object.create(null);
var state = null;
var currentStatus = 0;
var patchCalls = 0;
var saveStates = [];
var toasts = [];
var connectionMessages = [];
function resetState() {
  state = {
    credentials:{apiBase:"https://cpa.example",managementKey:"secret"},providers:[],
    config:{cache_ttl_seconds:300,provider_mappings:{}},draftMappings:{},draftTTL:"600",draftRevision:1,
    results:[],view:"settings",dirty:true,needsQuery:false,dataReady:true,
    saving:false,saveGeneration:0,saveController:null,loadGeneration:4,
    querying:false,queryGeneration:0,queryController:null
  };
  patchCalls = 0;
  saveStates = [];
  toasts = [];
  connectionMessages = [];
}
function byID() { return {}; }
function setButtonBusy() {}
function setText(_node, value) { saveStates.push(String(value || "")); }
function toast(message) { toasts.push(String(message || "")); }
function owns(value, key) { return Object.prototype.hasOwnProperty.call(value || {}, key); }
function stableMappings(value) { return JSON.stringify(Object.keys(value || {}).sort().map(function (key) { return [key,value[key]]; })); }
function normalizeConfig(value) { return {cache_ttl_seconds:Number(value.cache_ttl_seconds),provider_mappings:Object.assign({},value.provider_mappings || {})}; }
function mappedQueryType() { return ""; }
function resolvedProviderMapping() { return {key:"",type:""}; }
function refreshDirtyState() {
  state.dirty = String(state.draftTTL) !== String(state.config.cache_ttl_seconds) || stableMappings(state.draftMappings) !== stableMappings(state.config.provider_mappings);
  return state.dirty;
}
function apiFetch(_path, options) {
  if (options && options.method === "PATCH") { patchCalls += 1; return Promise.resolve({}); }
  return Promise.resolve({cache_ttl_seconds:300,provider_mappings:{}});
}
function waitForRuntimeConfig() {
  var error = new Error("管理密钥无效或权限不足");
  error.status = currentStatus;
  return Promise.reject(error);
}
function withConfigSaveLock(_credentials, _controller, callback) { return callback(function () {}); }
function showConnection(message) { connectionMessages.push(String(message || "")); }
function cancelQueryRequests() {}
function clearResultSnapshot() {}
function persistResultSnapshot() { return Promise.resolve(true); }
function buildAccounts() { return []; }
function resultsMatchAccounts() { return false; }
function renderSettings() {}
function renderResults() {}
function queryBalances() { return Promise.resolve(); }
var window = {setTimeout:function () {}};
` + page[start:end] + `
async function run(status) {
  resetState();
  currentStatus = status;
  var saved = await saveSettings();
  return {status:status,saved:saved,patch_calls:patchCalls,saving:state.saving,save_states:saveStates,toasts:toasts,connections:connectionMessages};
}
(async function () {
  process.stdout.write(JSON.stringify([await run(401),await run(403)]));
})().catch(function (error) { process.stderr.write(String(error && error.stack || error)); process.exit(1); });
`
	output, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("execute persisted auth-check save helper: %v\n%s", err, output)
	}
	var got []struct {
		Status      int      `json:"status"`
		Saved       bool     `json:"saved"`
		PatchCalls  int      `json:"patch_calls"`
		Saving      bool     `json:"saving"`
		SaveStates  []string `json:"save_states"`
		Toasts      []string `json:"toasts"`
		Connections []string `json:"connections"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode persisted auth-check save result: %v\n%s", err, output)
	}
	if len(got) != 2 {
		t.Fatalf("persisted auth-check cases = %d, want 2", len(got))
	}
	for _, result := range got {
		states := strings.Join(result.SaveStates, " ")
		messages := strings.Join(result.Toasts, " ")
		connections := strings.Join(result.Connections, " ")
		if !result.Saved || result.PatchCalls != 1 || result.Saving || !strings.Contains(states, "已保存") || !strings.Contains(messages, "设置已写入 CPA") || !strings.Contains(connections, "管理密钥已失效") {
			t.Fatalf("post-PATCH %d was reported as an ordinary save failure: %#v", result.Status, result)
		}
	}
}

func TestDashboardFailurePresentationUsesStructuredDetails(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	page := string(RenderDashboard(300))
	start := strings.Index(page, "  function failureKindLabel(kind)")
	end := strings.Index(page, "  function quotaPercent(item)")
	if start < 0 || end <= start {
		t.Fatal("cannot locate dashboard failure helpers")
	}
	script := `
function localizedError(value) { return String(value || ""); }
` + page[start:end] + `
process.stdout.write(JSON.stringify({
  rate:failurePresentation({error:"旧错误",failure:{kind:"rate_limited",title:"请求受限",reason:"达到频率限制",suggestion:"减少刷新频率",retry_after_seconds:12.2}}),
  duplicate_retry:failurePresentation({error:"旧错误",failure:{kind:"rate_limited",suggestion:"建议 12 秒后重试",retry_after_seconds:12}}).suggestion,
  partial_warning:failurePresentation({failure:resultWarnings({warnings:[{kind:"service_unavailable",title:"费用数据暂缺",reason:"费用接口暂时不可用",suggestion:"稍后重试"}]})[0]}),
  warning_count:resultWarnings({warnings:[null,"invalid",{kind:"timeout"}]}).length,
  balance:failureKindLabel("insufficient_funds"),
  quota:failureKindLabel("quota_exhausted"),
  network:failureKindLabel("dns"),
  unsupported:failureKindLabel("unsupported"),
  conflict:failureKindLabel("conflict")
}));
`
	output, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("execute dashboard failure helpers: %v\n%s", err, output)
	}
	var got struct {
		Rate struct {
			KindLabel  string `json:"kindLabel"`
			Title      string `json:"title"`
			Reason     string `json:"reason"`
			Suggestion string `json:"suggestion"`
		} `json:"rate"`
		DuplicateRetry string `json:"duplicate_retry"`
		PartialWarning struct {
			KindLabel  string `json:"kindLabel"`
			Title      string `json:"title"`
			Reason     string `json:"reason"`
			Suggestion string `json:"suggestion"`
		} `json:"partial_warning"`
		WarningCount int    `json:"warning_count"`
		Balance      string `json:"balance"`
		Quota        string `json:"quota"`
		Network      string `json:"network"`
		Unsupported  string `json:"unsupported"`
		Conflict     string `json:"conflict"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode dashboard failure result: %v\n%s", err, output)
	}
	if got.Rate.KindLabel != "请求限流" || got.Rate.Title != "请求受限" || got.Rate.Reason != "达到频率限制" || !strings.Contains(got.Rate.Suggestion, "建议 13 秒后重试") {
		t.Fatalf("structured rate-limit failure = %#v", got.Rate)
	}
	if strings.Count(got.DuplicateRetry, "秒后重试") != 1 {
		t.Fatalf("retry suggestion was duplicated: %q", got.DuplicateRetry)
	}
	if got.WarningCount != 1 || got.PartialWarning.KindLabel != "服务异常" || got.PartialWarning.Title != "费用数据暂缺" || got.PartialWarning.Reason != "费用接口暂时不可用" || !strings.Contains(got.PartialWarning.Suggestion, "稍后重试") {
		t.Fatalf("structured partial warning = %#v (count %d)", got.PartialWarning, got.WarningCount)
	}
	if got.Balance != "余额不足" || got.Quota != "额度已耗尽" || got.Network != "DNS 异常" || got.Unsupported != "暂不支持自动查询" || got.Conflict != "请求冲突" {
		t.Fatalf("failure kind labels = %#v", got)
	}
}
