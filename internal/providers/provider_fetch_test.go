package providers

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func useTestHTTPClient(t *testing.T, fn roundTripFunc) {
	t.Helper()
	original := httpClient
	httpClient = &http.Client{Transport: fn}
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
