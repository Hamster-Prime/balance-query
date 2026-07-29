package providers

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Hamster-Prime/balance-query/internal/balance"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func useTestHTTPClient(t *testing.T, fn roundTripFunc) {
	t.Helper()
	original := httpClient
	httpClient = &http.Client{Transport: fn, CheckRedirect: providerRedirectPolicy}
	t.Cleanup(func() { httpClient = original })
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestNewAPIFetchUsesAPIKeyBillingEndpoints(t *testing.T) {
	seen := map[string]bool{}
	useTestHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		if got := r.Header.Get("Authorization"); got != "Bearer new-api-key" {
			t.Errorf("Authorization = %q", got)
		}
		seen[r.URL.Path] = true
		switch r.URL.Path {
		case "/v1/dashboard/billing/subscription":
			return jsonResponse(`{"object":"billing_subscription","hard_limit_usd":100,"system_hard_limit_usd":100,"access_until":1800000000}`), nil
		case "/v1/dashboard/billing/usage":
			return jsonResponse(`{"object":"list","total_usage":2500}`), nil
		default:
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("not found"))}, nil
		}
	})

	result := (NewAPI{BaseURL: "https://new-api.example/v1"}).Fetch("new", "new-api-key", "")
	if result.Error != "" {
		t.Fatalf("NewAPI.Fetch() error = %q", result.Error)
	}
	if !seen["/v1/dashboard/billing/subscription"] || !seen["/v1/dashboard/billing/usage"] {
		t.Fatalf("requested paths = %#v", seen)
	}
	if len(result.QuotaWindows) != 1 {
		t.Fatalf("quota windows = %#v", result.QuotaWindows)
	}
	window := result.QuotaWindows[0]
	if window.Total != 100 || window.Used != 25 || window.Remaining != 75 {
		t.Fatalf("New API quota = %#v", window)
	}
	if window.AggregationScope != "unknown" {
		t.Fatalf("legacy New API scope = %q, want unknown", window.AggregationScope)
	}
	if result.ResetAt == "" || result.Extra["密钥到期"] != result.ResetAt {
		t.Fatalf("legacy New API expiry metadata = %#v", result)
	}
}

func TestNewAPILegacyUnlimitedSentinelShowsUsedOnly(t *testing.T) {
	useTestHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/usage/token/":
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("not found"))}, nil
		case "/v1/dashboard/billing/subscription":
			return jsonResponse(`{"hard_limit_usd":100000000,"system_hard_limit_usd":100000000}`), nil
		case "/v1/dashboard/billing/usage":
			return jsonResponse(`{"total_usage":2500}`), nil
		default:
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("not found"))}, nil
		}
	})

	result := (NewAPI{BaseURL: "https://new-api.example/v1"}).Fetch("new", "new-api-key", "")
	if result.Error != "" || len(result.QuotaWindows) != 1 {
		t.Fatalf("legacy unlimited result = %#v", result)
	}
	window := result.QuotaWindows[0]
	if !window.Unlimited || !window.ShowUsedWhenUnlimited || window.Used != 25 || window.Total != 0 || window.Remaining != 0 {
		t.Fatalf("legacy unlimited window = %#v", window)
	}
	if result.QuotaDisplay != "不限量，已用 25.0000 站点额度" {
		t.Fatalf("legacy unlimited display = %q", result.QuotaDisplay)
	}
}

func TestNewAPIFetchFallsBackToLegacyBillingOnMethodNotAllowed(t *testing.T) {
	seen := map[string]bool{}
	useTestHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		seen[r.URL.Path] = true
		switch r.URL.Path {
		case "/api/usage/token/":
			return &http.Response{StatusCode: http.StatusMethodNotAllowed, Body: io.NopCloser(strings.NewReader("method not allowed"))}, nil
		case "/v1/dashboard/billing/subscription":
			return jsonResponse(`{"hard_limit_usd":50}`), nil
		case "/v1/dashboard/billing/usage":
			return jsonResponse(`{"total_usage":1000}`), nil
		default:
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("not found"))}, nil
		}
	})

	result := (NewAPI{BaseURL: "https://new-api.example/v1"}).Fetch("new", "new-api-key", "")
	if result.Error != "" || !seen["/v1/dashboard/billing/subscription"] || !seen["/v1/dashboard/billing/usage"] {
		t.Fatalf("405 fallback result = %#v, paths = %#v", result, seen)
	}
}

func TestNewAPIFetchPrefersTokenUsageEndpoint(t *testing.T) {
	seen := map[string]bool{}
	useTestHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		seen[r.URL.Path] = true
		switch r.URL.Path {
		case "/api/usage/token/":
			if got := r.Header.Get("Authorization"); got != "Bearer new-api-key" {
				t.Fatalf("Authorization = %q", got)
			}
			return jsonResponse(`{"code":true,"message":"ok","data":{"object":"token_usage","name":"primary","total_granted":5000000,"total_used":1000000,"total_available":4000000,"unlimited_quota":false,"model_limits_enabled":false,"expires_at":0}}`), nil
		case "/api/status":
			return jsonResponse(`{"success":true,"data":{"quota_per_unit":500000,"quota_display_type":"USD"}}`), nil
		default:
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("not found"))}, nil
		}
	})

	result := (NewAPI{BaseURL: "https://new-api.example/v1"}).Fetch("new", "new-api-key", "")
	if result.Error != "" {
		t.Fatalf("NewAPI.Fetch() error = %q", result.Error)
	}
	if !seen["/api/usage/token/"] || !seen["/api/status"] || seen["/v1/dashboard/billing/subscription"] {
		t.Fatalf("requested paths = %#v", seen)
	}
	if got := result.QuotaWindows[0]; got.Total != 10 || got.Used != 2 || got.Remaining != 8 || got.Unit != "USD" {
		t.Fatalf("preferred token usage window = %#v", got)
	}
	if got := result.QuotaWindows[0].AggregationScope; got != "key" {
		t.Fatalf("token usage scope = %q, want key", got)
	}
}

func TestNewAPIFetchRejectsSuccessfulPayloadWithoutQuotaFields(t *testing.T) {
	useTestHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/api/usage/token/" {
			return jsonResponse(`{"code":true,"message":"ok","data":{"object":"token_usage","name":"incomplete"}}`), nil
		}
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("not found"))}, nil
	})

	result := (NewAPI{BaseURL: "https://new-api.example/v1"}).Fetch("new", "new-api-key", "")
	if result.Error != "New API 未返回密钥额度字段" {
		t.Fatalf("missing quota fields result = %#v", result)
	}
}

func TestNewAPIFetchRejectsNullOrPartialQuotaFields(t *testing.T) {
	for name, payload := range map[string]string{
		"null fields":    `{"code":true,"data":{"total_granted":null,"total_used":null,"total_available":null,"unlimited_quota":false}}`,
		"partial fields": `{"code":true,"data":{"total_granted":100,"total_used":25,"unlimited_quota":false}}`,
	} {
		t.Run(name, func(t *testing.T) {
			useTestHTTPClient(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/api/usage/token/" {
					return jsonResponse(payload), nil
				}
				return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("not found"))}, nil
			})
			result := (NewAPI{BaseURL: "https://new-api.example/v1"}).Fetch("new", "new-api-key", "")
			if result.Error != "New API 未返回密钥额度字段" {
				t.Fatalf("invalid quota payload result = %#v", result)
			}
		})
	}
}

func TestNewAPIUnlimitedWithoutUsageDoesNotInventZeroUsed(t *testing.T) {
	useTestHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/api/usage/token/" {
			return jsonResponse(`{"code":true,"data":{"unlimited_quota":true}}`), nil
		}
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("not found"))}, nil
	})
	result := (NewAPI{BaseURL: "https://new-api.example/v1"}).Fetch("new", "new-api-key", "")
	if result.Error != "" || len(result.QuotaWindows) != 1 {
		t.Fatalf("unlimited result = %#v", result)
	}
	if result.QuotaWindows[0].ShowUsedWhenUnlimited || result.QuotaDisplay != "不限量" {
		t.Fatalf("unlimited result invented usage = %#v", result)
	}
}

func TestMiniMaxGlobalRetriesBusinessFailureWithXAPIKey(t *testing.T) {
	requests := 0
	useTestHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		requests++
		if r.URL.String() != "https://api.minimax.io/v1/token_plan/remains" {
			t.Fatalf("request URL = %q", r.URL.String())
		}
		if requests == 1 {
			if got := r.Header.Get("Authorization"); got != "Bearer minimax-key" {
				t.Fatalf("Authorization = %q", got)
			}
			return jsonResponse(`{"base_resp":{"status_code":1004,"status_msg":"credential style rejected"}}`), nil
		}
		if got := r.Header.Get("x-api-key"); got != "minimax-key" {
			t.Fatalf("x-api-key = %q", got)
		}
		return jsonResponse(`{"base_resp":{"status_code":0},"model_remains":[{"model_name":"general","current_interval_total_count":100,"current_interval_usage_count":0,"current_interval_status":2,"current_weekly_total_count":1000,"current_weekly_usage_count":0,"current_weekly_status":2}]}`), nil
	})

	result := (MiniMaxCodingGlobal{}).Fetch("minimax-global", "minimax-key", "")
	if result.Error != "" || requests != 2 {
		t.Fatalf("result = %#v, requests = %d", result, requests)
	}
	if len(result.QuotaWindows) != 2 || result.QuotaWindows[0].Status != "已用尽" || result.QuotaWindows[0].Remaining != 0 {
		t.Fatalf("depleted MiniMax quota = %#v", result.QuotaWindows)
	}
}

func TestMiniMaxEmptyQuotaResponseIsSuccessful(t *testing.T) {
	useTestHTTPClient(t, func(*http.Request) (*http.Response, error) {
		return jsonResponse(`{"base_resp":{"status_code":0},"data":{"current_subscribe_title":"Token Plan Plus","points_balance":"42","model_remains":[]}}`), nil
	})

	result := (MiniMaxCodingGlobal{}).Fetch("minimax-global", "minimax-key", "")
	if result.Error != "" || len(result.QuotaWindows) != 1 || !result.QuotaWindows[0].Unavailable {
		t.Fatalf("empty MiniMax quota result = %#v", result)
	}
	if result.Plan != "Token Plan Plus" || result.Extra["积分余额"] != "42" {
		t.Fatalf("empty MiniMax metadata = %#v", result)
	}
}

func TestMiniMaxMissingBusinessStatusRetriesXAPIKey(t *testing.T) {
	requests := 0
	useTestHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		requests++
		if requests == 1 {
			return jsonResponse(`{"error":"unauthorized"}`), nil
		}
		if got := r.Header.Get("x-api-key"); got != "minimax-key" {
			t.Fatalf("x-api-key = %q", got)
		}
		return jsonResponse(`{"base_resp":{"status_code":0},"model_remains":[{"model_name":"general","current_interval_total_count":100,"current_interval_usage_count":80,"current_interval_status":1,"current_weekly_total_count":1000,"current_weekly_usage_count":900,"current_weekly_status":1}]}`), nil
	})

	result := (MiniMaxCodingGlobal{}).Fetch("minimax-global", "minimax-key", "")
	if result.Error != "" || requests != 2 || len(result.QuotaWindows) != 2 {
		t.Fatalf("missing-status fallback result = %#v, requests = %d", result, requests)
	}
}

func TestSub2APIFetchDecodesTopLevelUsage(t *testing.T) {
	useTestHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/usage" {
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("not found"))}, nil
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sub-key" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.URL.Query().Get("days"); got != "30" {
			t.Errorf("days = %q, want 30", got)
		}
		return jsonResponse(`{"mode":"quota_limited","isValid":true,"quota":{"limit":20,"used":8,"remaining":12,"unit":"USD"},"rate_limits":[{"window":"5h","limit":10,"used":2,"remaining":8}]}`), nil
	})

	result := (Sub2API{BaseURL: "https://sub2api.example/v1"}).Fetch("sub", "sub-key", "")
	if result.Error != "" {
		t.Fatalf("Sub2API.Fetch() error = %q", result.Error)
	}
	if len(result.QuotaWindows) != 2 || result.QuotaWindows[1].Label != "5 小时额度" {
		t.Fatalf("Sub2API quota windows = %#v", result.QuotaWindows)
	}
	if result.QuotaDisplay != "总额度：剩余 12 / 20 USD" {
		t.Fatalf("Sub2API quota display = %q", result.QuotaDisplay)
	}
	for _, window := range result.QuotaWindows {
		if window.AggregationScope != "key" {
			t.Fatalf("Sub2API key window scope = %q, want key: %#v", window.AggregationScope, window)
		}
	}
}

func TestSub2APIFetchFallsBackToXAPIKey(t *testing.T) {
	requests := 0
	useTestHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		requests++
		if requests == 1 {
			return &http.Response{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader("unauthorized"))}, nil
		}
		if got := r.Header.Get("x-api-key"); got != "sub-key" {
			t.Fatalf("x-api-key = %q", got)
		}
		return jsonResponse(`{"mode":"unrestricted","isValid":true,"balance":12.5,"remaining":12.5,"unit":"USD","planName":"钱包余额"}`), nil
	})

	result := (Sub2API{BaseURL: "https://sub2api.example/v1"}).Fetch("sub", "sub-key", "")
	if result.Error != "" || requests != 2 {
		t.Fatalf("result = %#v, requests = %d", result, requests)
	}
	if !result.HasBalance || result.BalanceScope != "account" || result.BalanceUSD != 12.5 ||
		!result.HasBalanceAmount || result.BalanceAmount != 12.5 || result.BalanceCurrency != "USD" {
		t.Fatalf("Sub2API balance metadata = %#v", result)
	}
}

func TestDeepSeekFetchPreservesZeroUSDBalance(t *testing.T) {
	useTestHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/user/balance" {
			t.Fatalf("request path = %q", r.URL.Path)
		}
		return jsonResponse(`{"is_available":true,"balance_infos":[{"currency":"USD","total_balance":"0","granted_balance":"0","topped_up_balance":"0"}]}`), nil
	})

	result := (DeepSeek{BaseURL: "https://api.deepseek.com"}).Fetch("deepseek", "deepseek-key", "")
	if result.Error != "" {
		t.Fatalf("DeepSeek.Fetch() error = %q", result.Error)
	}
	if !result.HasBalance || result.BalanceUSD != 0 || !result.HasBalanceAmount || result.BalanceAmount != 0 ||
		result.BalanceCurrency != "USD" || result.BalanceScope != "account" {
		t.Fatalf("zero USD balance metadata = %#v", result)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal zero balance: %v", err)
	}
	if !strings.Contains(string(raw), `"has_balance":true`) || !strings.Contains(string(raw), `"balance_scope":"account"`) {
		t.Fatalf("zero balance presence metadata was lost in JSON: %s", raw)
	}
}

func TestKimiAPIFetchKeepsCNYOutOfUSDBalanceField(t *testing.T) {
	useTestHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://api.moonshot.cn/v1/users/me/balance" {
			t.Errorf("request URL = %q", r.URL.String())
		}
		if got := r.Header.Get("Authorization"); got != "Bearer moonshot-key" {
			t.Errorf("Authorization = %q", got)
		}
		return jsonResponse(`{"code":0,"status":true,"data":{"available_balance":12.5,"cash_balance":10,"voucher_balance":2.5}}`), nil
	})

	result := (KimiAPI{BaseURL: "https://api.moonshot.cn/v1"}).Fetch("kimi-api", "moonshot-key", "")
	if result.Error != "" {
		t.Fatalf("KimiAPI.Fetch() error = %q", result.Error)
	}
	if result.BalanceUSD != 0 {
		t.Fatalf("BalanceUSD = %v, CNY must not be stored as USD", result.BalanceUSD)
	}
	if !result.HasBalanceAmount || result.BalanceAmount != 12.5 || result.BalanceCurrency != "CNY" || result.BalanceScope != "account" {
		t.Fatalf("typed CNY balance = %#v", result)
	}
	if result.QuotaDisplay != "可用 12.5000 元" {
		t.Fatalf("quota display = %q", result.QuotaDisplay)
	}
}

func TestKimiAPIFetchRejectsAnyBusinessFailureSignal(t *testing.T) {
	for _, response := range []string{
		`{"code":1,"status":true,"scode":"bad-code","data":{"available_balance":99}}`,
		`{"code":0,"status":false,"scode":"bad-status","data":{"available_balance":99}}`,
	} {
		t.Run(response, func(t *testing.T) {
			useTestHTTPClient(t, func(*http.Request) (*http.Response, error) {
				return jsonResponse(response), nil
			})
			result := (KimiAPI{BaseURL: "https://api.moonshot.cn/v1"}).Fetch("kimi-api", "moonshot-key", "")
			if result.Error == "" {
				t.Fatalf("KimiAPI.Fetch() accepted failure payload: %s", response)
			}
		})
	}
}

func TestClaudeAdminFetchAggregatesUsageAndCosts(t *testing.T) {
	seen := map[string]bool{}
	var seenMu sync.Mutex
	useTestHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		if got := r.Header.Get("x-api-key"); got != "sk-ant-admin-test" {
			t.Errorf("x-api-key = %q", got)
		}
		if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
			t.Errorf("anthropic-version = %q", got)
		}
		if got := r.URL.Query().Get("bucket_width"); got != "1d" {
			t.Errorf("bucket_width = %q", got)
		}
		if r.URL.Query().Get("starting_at") == "" || r.URL.Query().Get("ending_at") == "" {
			t.Errorf("missing 30-day time range: %s", r.URL.RawQuery)
		}
		seenMu.Lock()
		seen[r.URL.Path] = true
		seenMu.Unlock()
		switch r.URL.Path {
		case "/custom/v1/organizations/usage_report/messages":
			if got := r.URL.Query()["group_by[]"]; len(got) != 1 || got[0] != "model" {
				t.Errorf("usage group_by = %#v", got)
			}
			return jsonResponse(`{"data":[{"starting_at":"2026-07-01T00:00:00Z","ending_at":"2026-07-02T00:00:00Z","results":[{"uncached_input_tokens":100,"cache_read_input_tokens":20,"cache_creation":{"ephemeral_1h_input_tokens":10,"ephemeral_5m_input_tokens":5},"output_tokens":50,"model":"claude-sonnet-4-5","server_tool_use":{"web_search_requests":2}},{"uncached_input_tokens":30,"cache_read_input_tokens":0,"cache_creation":{"ephemeral_1h_input_tokens":0,"ephemeral_5m_input_tokens":0},"output_tokens":10,"model":"claude-haiku-4-5","server_tool_use":{"web_search_requests":0}}]}],"has_more":false}`), nil
		case "/custom/v1/organizations/cost_report":
			if got := r.URL.Query()["group_by[]"]; len(got) != 1 || got[0] != "description" {
				t.Errorf("cost group_by = %#v", got)
			}
			return jsonResponse(`{"data":[{"starting_at":"2026-07-01T00:00:00Z","ending_at":"2026-07-02T00:00:00Z","results":[{"amount":"123.45","currency":"USD","cost_type":"tokens","description":"tokens"},{"amount":"50","currency":"USD","cost_type":"web_search","description":"web_search"}]}],"has_more":false}`), nil
		default:
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("not found"))}, nil
		}
	})

	result := (ClaudeAdmin{BaseURL: "https://claude.example/custom/v1"}).Fetch("claude", "sk-ant-admin-test", "")
	if result.Error != "" {
		t.Fatalf("ClaudeAdmin.Fetch() error = %q", result.Error)
	}
	seenMu.Lock()
	seenUsage := seen["/custom/v1/organizations/usage_report/messages"]
	seenCost := seen["/custom/v1/organizations/cost_report"]
	seenMu.Unlock()
	if !seenUsage || !seenCost {
		t.Fatalf("requested paths = %#v", seen)
	}
	if result.QuotaDisplay != "近 30 天费用 1.73 USD" {
		t.Fatalf("quota display = %q", result.QuotaDisplay)
	}
	if !result.HasCost || result.CostUSD != 1.7345 || result.CostScope != "organization" {
		t.Fatalf("Claude cost metadata = %#v", result)
	}
	if got := result.Extra["近 30 天总令牌"]; got != "225" {
		t.Fatalf("total tokens = %q", got)
	}
	if got := result.Extra["Web 搜索请求"]; got != "2 次" {
		t.Fatalf("web searches = %q", got)
	}
	if got := result.Extra["费用 令牌"]; got != "1.23 USD" {
		t.Fatalf("token cost = %q", got)
	}
	if result.BalanceUSD != 0 || len(result.QuotaWindows) != 0 {
		t.Fatalf("historical report must not become balance/quota: %#v", result)
	}
}

func TestClaudeAdminFetchAllowsPartialSuccess(t *testing.T) {
	useTestHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/usage_report/messages") {
			return jsonResponse(`{"data":[{"results":[{"uncached_input_tokens":40,"output_tokens":10,"model":"claude-sonnet-4-5"}]}],"has_more":false}`), nil
		}
		return &http.Response{StatusCode: http.StatusForbidden, Body: io.NopCloser(strings.NewReader("forbidden"))}, nil
	})

	result := (ClaudeAdmin{BaseURL: "https://api.anthropic.com"}).Fetch("claude", "sk-ant-admin-test", "")
	if result.Error != "" {
		t.Fatalf("partial success returned error = %q", result.Error)
	}
	if result.QuotaDisplay != "近 30 天使用 50 令牌" {
		t.Fatalf("quota display = %q", result.QuotaDisplay)
	}
	if !strings.Contains(result.Extra["费用查询"], "未成功") {
		t.Fatalf("cost query detail = %q", result.Extra["费用查询"])
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
	warning := result.Warnings[0]
	if warning.Kind != balance.FailurePermission || warning.HTTPStatus != http.StatusForbidden || !strings.Contains(warning.Title, "费用查询") {
		t.Fatalf("warning = %#v", warning)
	}
}

func TestClaudeAdminFetchesUsageAndCostConcurrently(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseRequests := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseRequests()

	useTestHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		started <- r.URL.Path
		<-release
		if strings.HasSuffix(r.URL.Path, "/usage_report/messages") {
			return jsonResponse(`{"data":[],"has_more":false}`), nil
		}
		return jsonResponse(`{"data":[],"has_more":false}`), nil
	})

	done := make(chan balance.Result, 1)
	go func() {
		done <- (ClaudeAdmin{BaseURL: "https://api.anthropic.com"}).Fetch("claude", "sk-ant-admin-test", "")
	}()

	for requestCount := 0; requestCount < 2; requestCount++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("usage and cost requests did not start concurrently")
		}
	}
	releaseRequests()
	select {
	case result := <-done:
		if result.Error != "" {
			t.Fatalf("result error = %q", result.Error)
		}
	case <-time.After(time.Second):
		t.Fatal("ClaudeAdmin.Fetch did not finish")
	}
}

func TestClaudeAdminFetchFailsOnlyWhenBothEndpointsFail(t *testing.T) {
	useTestHTTPClient(t, func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader("unauthorized"))}, nil
	})

	result := (ClaudeAdmin{BaseURL: "https://api.anthropic.com"}).Fetch("claude", "standard-key", "")
	if result.Error == "" || !strings.Contains(result.Error, "用量查询失败") || !strings.Contains(result.Error, "费用查询失败") {
		t.Fatalf("result error = %q", result.Error)
	}
	if result.Failure == nil || result.Failure.Kind != balance.FailureAuthentication || result.Failure.HTTPStatus != http.StatusUnauthorized {
		t.Fatalf("result failure = %#v", result.Failure)
	}
}

func TestClaudeCostDetailLabelsAreLocalized(t *testing.T) {
	tests := []struct {
		item claudeCostResult
		want string
	}{
		{item: claudeCostResult{CostType: "tokens", Model: "claude-sonnet", TokenType: "uncached_input_tokens"}, want: "claude-sonnet · 输入令牌"},
		{item: claudeCostResult{Description: "Web Search Usage"}, want: "Web 搜索"},
		{item: claudeCostResult{Description: "Code Execution Usage"}, want: "代码执行"},
		{item: claudeCostResult{Description: "unrecognized upstream label"}, want: "其他费用"},
	}
	for _, test := range tests {
		if got := claudeCostDetailLabel(test.item); got != test.want {
			t.Errorf("claudeCostDetailLabel(%#v) = %q, want %q", test.item, got, test.want)
		}
	}
}
