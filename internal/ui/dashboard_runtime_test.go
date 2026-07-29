package ui

import (
	"encoding/json"
	"math"
	"os/exec"
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
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := runDashboardQuotaResultState(t, test.windows); got != test.want {
				t.Fatalf("quota result state = %q, want %q", got, test.want)
			}
		})
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
	if start < 0 || end <= start {
		t.Fatal("cannot locate dashboard snapshot helpers")
	}
	script := `
var QUERY_BATCH_SIZE = 128;
var RESULT_SNAPSHOT_KEY = "balance-query-results::v1";
var RESULT_SNAPSHOT_VERSION = 1;
var RESULT_SNAPSHOT_MAX_BYTES = 2 * 1024 * 1024;
var RESULT_SNAPSHOT_MAX_STALE_MS = 7 * 24 * 60 * 60 * 1000;
var SECRET_SALT = "cli-proxy-api-webui::secure-storage";
var state = { snapshotGeneration:0 };
var memory = Object.create(null);
var storage = {
  getItem:function (key) { return Object.prototype.hasOwnProperty.call(memory, key) ? memory[key] : null; },
  setItem:function (key, value) { memory[key] = String(value); },
  removeItem:function (key) { delete memory[key]; }
};
var window = { crypto:null, sessionStorage:storage, location:{origin:"https://cpa.example",host:"cpa.example"} };
window.parent = window;
function maskKey(value) { return "masked-" + String(value).slice(-2); }
var now = Date.parse("2099-07-29T00:00:00Z");
Date.now = function () { return now; };
` + page[start:end] + `
(async function () {
  var credentials = {apiBase:"https://cpa.example",managementKey:"management-secret"};
  var accounts = [{
    id:"account-1",provider_key:"provider-1",mapping_key:"provider-1",account_name:"账户一",
    base_url:"https://provider.example/v1",proxy_url:"",query_type:"newapi",api_key:"sk-provider-secret"
  }];
  var results = [{account_name:"账户一",fetched_at:"2099-07-29T00:00:00Z",extra:{note:"sk-provider-secret / management-secret"}}];
  var wrote = await persistResultSnapshot(accounts, results, 300, credentials);
  var serialized = storage.getItem(RESULT_SNAPSHOT_KEY) || "";
  var restored = await readResultSnapshot(accounts, 300, credentials);
  var warningResults = [{
    account_name:"账户一",fetched_at:"2099-07-29T00:00:00Z",balance:12.5,
    warnings:[{kind:"service_unavailable",title:"费用数据暂缺",reason:"sk-provider-secret / management-secret",suggestion:"稍后重试"}]
  }];
  await persistResultSnapshot(accounts, warningResults, 300, credentials);
  var warningSerialized = storage.getItem(RESULT_SNAPSHOT_KEY) || "";
  var warningRestored = await readResultSnapshot(accounts, 300, credentials);
  await persistResultSnapshot(accounts, results, 300, credentials);
  now += 301000;
  var stale = await readResultSnapshot(accounts, 300, credentials);
  var changed = JSON.parse(JSON.stringify(accounts));
  changed[0].api_key = "sk-provider-changed";
  var mismatch = await readResultSnapshot(changed, 300, credentials);
  await persistResultSnapshot(accounts, results, 300, credentials);
  await readResultSnapshot(accounts, 0, credentials);
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
    stale_fresh:stale && stale.fresh,
    mismatch:mismatch,
    cleared:storage.getItem(RESULT_SNAPSHOT_KEY) == null,
    fallback_sha:fallbackSHA256("abc")
  }));
})().catch(function (error) { process.stderr.write(String(error && error.stack || error)); process.exit(1); });
`
	output, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("execute dashboard snapshot helpers: %v\n%s", err, output)
	}
	var got struct {
		Wrote                 bool   `json:"wrote"`
		ContainsAPIKey        bool   `json:"contains_api_key"`
		ContainsManagementKey bool   `json:"contains_management_key"`
		RestoredFresh         bool   `json:"restored_fresh"`
		RestoredComplete      bool   `json:"restored_complete"`
		RestoredNote          string `json:"restored_note"`
		WarningContainsAPIKey bool   `json:"warning_contains_api_key"`
		WarningContainsMgmt   bool   `json:"warning_contains_management_key"`
		WarningFresh          bool   `json:"warning_fresh"`
		WarningComplete       bool   `json:"warning_complete"`
		WarningCount          int    `json:"warning_count"`
		WarningReason         string `json:"warning_reason"`
		StaleFresh            bool   `json:"stale_fresh"`
		Mismatch              any    `json:"mismatch"`
		Cleared               bool   `json:"cleared"`
		FallbackSHA           string `json:"fallback_sha"`
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
	if got.StaleFresh || got.Mismatch != nil || !got.Cleared {
		t.Fatalf("snapshot TTL/fingerprint invalidation failed: %#v", got)
	}
	if got.FallbackSHA != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("fallback SHA-256 = %q", got.FallbackSHA)
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
  queryController:null,queryGeneration:0,querying:false
};
var currentAccounts = [{id:"request-old"}];
var deferred = [];
var buttonStates = [];
var persisted = [];
var resultNode = { setAttribute:function () {} };
function buildAccounts() { return currentAccounts.slice(); }
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

func TestDashboardSaveIgnoresResponseAfterReconnectWithSameCredentials(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	page := string(RenderDashboard(300))
	cancelStart := strings.Index(page, "  function cancelLoadRequests()")
	cancelEnd := strings.Index(page, "  function showConnection(message)")
	saveStart := strings.Index(page, "  function validateTTL()")
	saveEnd := strings.Index(page, "  function loadData()")
	if cancelStart < 0 || cancelEnd <= cancelStart || saveStart < 0 || saveEnd <= saveStart {
		t.Fatal("cannot locate dashboard save request helpers")
	}
	script := `
var REQUEST_TIMEOUT = 30000;
var providerLabels = Object.create(null);
var state = {
  credentials:{apiBase:"https://cpa.example",managementKey:"same-secret"},
  providers:[],config:{cache_ttl_seconds:300,provider_mappings:{}},draftMappings:{},draftTTL:"600",
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
function resolvedProviderMapping() { return {key:"",type:""}; }
function apiFetch(_path, options) {
  requestSignal = options.signal;
  return new Promise(function (resolve) { resolveRequest = resolve; });
}
function clearResultSnapshot() {}
function persistResultSnapshot() { persistCount += 1; return Promise.resolve(true); }
function buildAccounts() { return []; }
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
