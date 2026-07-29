package providers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Hamster-Prime/balance-query/internal/balance"
)

func TestProviderHTTPErrorClassificationAndMetadata(t *testing.T) {
	tests := []struct {
		name           string
		status         int
		body           string
		headers        http.Header
		wantKind       string
		wantCode       string
		wantRequestID  string
		wantRetryAfter int64
	}{
		{
			name:     "nested MiniMax authentication",
			status:   http.StatusBadRequest,
			body:     `{"data":{"base_resp":{"status_code":1004,"status_msg":"invalid key"}}}`,
			wantKind: balance.FailureAuthentication,
			wantCode: "1004",
		},
		{
			name:     "conflict",
			status:   http.StatusConflict,
			body:     `{"message":"request conflict"}`,
			wantKind: balance.FailureConflict,
		},
		{
			name:          "authentication",
			status:        http.StatusUnauthorized,
			body:          `{"error":{"type":"invalid_authentication_error","message":"invalid key"},"request_id":"req-body"}`,
			wantKind:      balance.FailureAuthentication,
			wantCode:      "invalid_authentication_error",
			wantRequestID: "req-body",
		},
		{
			name:     "insufficient funds",
			status:   http.StatusPaymentRequired,
			body:     `{"code":"insufficient_balance","message":"please recharge"}`,
			wantKind: balance.FailureInsufficientFund,
			wantCode: "insufficient_balance",
		},
		{
			name:     "permission",
			status:   http.StatusForbidden,
			body:     `{"code":"ACCESS_DENIED","message":"IP not allowed"}`,
			wantKind: balance.FailurePermission,
			wantCode: "ACCESS_DENIED",
		},
		{
			name:     "endpoint hides non JSON body",
			status:   http.StatusNotFound,
			body:     `<html>secret-upstream-page</html>`,
			wantKind: balance.FailureEndpoint,
		},
		{
			name:           "rate limit metadata",
			status:         http.StatusTooManyRequests,
			body:           `{"error":{"type":"rate_limit_reached_error","message":"slow down"}}`,
			headers:        http.Header{"Retry-After": []string{"17"}, "X-Request-Id": []string{"req-header"}},
			wantKind:       balance.FailureRateLimited,
			wantCode:       "rate_limit_reached_error",
			wantRequestID:  "req-header",
			wantRetryAfter: 17,
		},
		{
			name:     "service unavailable",
			status:   http.StatusServiceUnavailable,
			body:     `{"base_resp":{"status_code":1033,"status_msg":"server busy"}}`,
			wantKind: balance.FailureService,
			wantCode: "1033",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			useTestHTTPClient(t, func(*http.Request) (*http.Response, error) {
				headers := test.headers.Clone()
				if headers == nil {
					headers = make(http.Header)
				}
				headers.Set("Content-Type", "application/json")
				return &http.Response{
					StatusCode: test.status,
					Header:     headers,
					Body:       io.NopCloser(strings.NewReader(test.body)),
				}, nil
			})

			var payload map[string]any
			err := getJSON("https://provider.example/usage", "test-key", "", &payload)
			var providerErr *ProviderError
			if !errors.As(err, &providerErr) {
				t.Fatalf("error = %T %v, want *ProviderError", err, err)
			}
			if providerErr.Kind != test.wantKind || providerErr.HTTPStatus != test.status || providerErr.ProviderCode != test.wantCode {
				t.Fatalf("ProviderError = %#v", providerErr)
			}
			if providerErr.RequestID != test.wantRequestID || providerErr.RetryAfterSeconds != test.wantRetryAfter {
				t.Fatalf("ProviderError metadata = %#v", providerErr)
			}
			if strings.Contains(err.Error(), "secret-upstream-page") {
				t.Fatalf("non-JSON response body leaked into error: %q", err.Error())
			}

			result := errResult("auth", "provider", err)
			if result.Failure == nil || result.Failure.Kind != test.wantKind || result.Failure.HTTPStatus != test.status {
				t.Fatalf("failure result = %#v", result)
			}
			if result.Failure.ProviderCode != test.wantCode || result.Failure.RequestID != test.wantRequestID || result.Failure.RetryAfterSeconds != test.wantRetryAfter {
				t.Fatalf("failure metadata = %#v", result.Failure)
			}
			if test.wantRetryAfter > 0 && !result.Failure.Retryable {
				t.Fatalf("Retry-After failure is not retryable: %#v", result.Failure)
			}
		})
	}
}

func TestHTTPErrorRedactsLongCredentialBeforeTruncation(t *testing.T) {
	key := "sk-" + strings.Repeat("sensitive", 40)
	useTestHTTPClient(t, func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusUnauthorized,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"message":"invalid key ` + key + `"}`)),
		}, nil
	})
	var payload map[string]any
	err := getJSON("https://provider.example/usage", key, "", &payload)
	if err == nil || strings.Contains(err.Error(), key[:120]) || !strings.Contains(err.Error(), "[已隐藏]") {
		t.Fatalf("credential was not safely redacted: %q", err)
	}
}

func TestCrossOriginRedirectIsNotFollowed(t *testing.T) {
	requests := 0
	useTestHTTPClient(t, func(*http.Request) (*http.Response, error) {
		requests++
		return &http.Response{
			StatusCode: http.StatusFound,
			Header:     http.Header{"Location": []string{"https://attacker.example/collect"}},
			Body:       io.NopCloser(strings.NewReader(`{"message":"moved"}`)),
		}, nil
	})
	var payload map[string]any
	err := getJSON("https://provider.example/usage", "secret-key", "", &payload)
	var providerErr *ProviderError
	if requests != 1 || !errors.As(err, &providerErr) || providerErr.Kind != balance.FailureEndpoint || providerErr.HTTPStatus != http.StatusFound {
		t.Fatalf("redirect result: requests=%d err=%#v", requests, err)
	}
}

func TestProxyDoesNotHideTLSFailure(t *testing.T) {
	err := transportProviderError(errors.New("x509: certificate has expired"), true)
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || providerErr.Kind != balance.FailureTLS {
		t.Fatalf("TLS error = %#v", err)
	}
}

func TestProxyDNSFailureIsClassifiedAsProxy(t *testing.T) {
	dnsErr := &net.DNSError{Err: "no such host", Name: "proxy.example"}
	proxied := errResult("auth", "provider", transportProviderError(dnsErr, true))
	if proxied.Failure == nil || proxied.Failure.Kind != balance.FailureProxy {
		t.Fatalf("proxied DNS failure = %#v", proxied)
	}
	direct := errResult("auth", "provider", transportProviderError(dnsErr, false))
	if direct.Failure == nil || direct.Failure.Kind != balance.FailureDNS {
		t.Fatalf("direct DNS failure = %#v", direct)
	}
}

func TestOversizedHTTPErrorKeepsStatusAndHeaders(t *testing.T) {
	useTestHTTPClient(t, func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header: http.Header{
				"Retry-After":  []string{"23"},
				"X-Request-Id": []string{"req-oversized"},
			},
			Body: io.NopCloser(strings.NewReader(strings.Repeat("x", int(maxProviderResponseBytes+1)))),
		}, nil
	})
	var payload map[string]any
	err := getJSON("https://provider.example/usage", "key", "", &payload)
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || providerErr.Kind != balance.FailureRateLimited ||
		providerErr.HTTPStatus != http.StatusTooManyRequests || providerErr.RetryAfterSeconds != 23 || providerErr.RequestID != "req-oversized" {
		t.Fatalf("oversized HTTP error = %#v", err)
	}
}

func TestClaudeReportContextDeadlineIsTypedTimeout(t *testing.T) {
	useTestHTTPClient(t, func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := fetchClaudeUsageWithContext(ctx, "https://api.anthropic.com/v1/organizations/usage_report/messages", url.Values{}, "", nil)
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || providerErr.Kind != balance.FailureTimeout {
		t.Fatalf("deadline error = %#v", err)
	}
}

func TestHTTPStatusMatchingUsesTypedStatusOnly(t *testing.T) {
	err := &ProviderError{HTTPStatus: http.StatusInternalServerError, Message: "body mentioned HTTP 404"}
	if isHTTPStatusError(err, http.StatusNotFound) {
		t.Fatal("HTTP 500 error was incorrectly matched as 404")
	}
	if !isHTTPStatusError(err, http.StatusInternalServerError) {
		t.Fatal("typed HTTP 500 status was not matched")
	}
}

func TestMiniMaxOnlyRetriesAuthenticationFailures(t *testing.T) {
	tests := []struct {
		name     string
		response func() *http.Response
		wantKind string
	}{
		{
			name: "HTTP rate limit",
			response: func() *http.Response {
				return &http.Response{
					StatusCode: http.StatusTooManyRequests,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"error":{"type":"rate_limit_reached_error","message":"slow down"}}`)),
				}
			},
			wantKind: balance.FailureRateLimited,
		},
		{
			name: "business rate limit",
			response: func() *http.Response {
				return jsonResponse(`{"base_resp":{"status_code":1002,"status_msg":"rate limit reached"}}`)
			},
			wantKind: balance.FailureRateLimited,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requests := 0
			useTestHTTPClient(t, func(*http.Request) (*http.Response, error) {
				requests++
				return test.response(), nil
			})
			result := (MiniMaxCodingGlobal{}).Fetch("mini", "mini-key", "")
			if requests != 1 {
				t.Fatalf("requests = %d, want 1", requests)
			}
			if result.Failure == nil || result.Failure.Kind != test.wantKind {
				t.Fatalf("result = %#v", result)
			}
		})
	}
}

func TestMiniMaxRetriesUnderscoreAuthenticationCode(t *testing.T) {
	requests := 0
	useTestHTTPClient(t, func(request *http.Request) (*http.Response, error) {
		requests++
		if requests == 1 {
			return jsonResponse(`{"error":"invalid_api_key"}`), nil
		}
		if request.Header.Get("x-api-key") == "" {
			t.Fatal("alternate x-api-key authentication was not used")
		}
		return jsonResponse(`{"base_resp":{"status_code":0},"model_remains":[]}`), nil
	})
	result := (MiniMaxCodingGlobal{}).Fetch("mini", "key", "")
	if result.Error != "" || requests != 2 {
		t.Fatalf("result=%#v requests=%d", result, requests)
	}
}

func TestKimiCodeRejectsPayloadWithoutQuotaFields(t *testing.T) {
	for _, response := range []kimiUsageResp{
		{},
		{Usage: map[string]any{"resetTime": "2026-07-29T10:00:00Z"}},
		{Usage: map[string]any{"limit": "not-a-number"}},
		{Limits: []map[string]any{{"detail": map[string]any{"resetTime": "2026-07-29T10:00:00Z"}}}},
		{BoosterWallet: map[string]any{"balance": map[string]any{"type": "BOOSTER"}}},
		{BoosterWallet: map[string]any{"balance": map[string]any{"type": "OTHER", "amount": float64(100)}}},
	} {
		result := parseKimiUsage("kimi", response)
		if result.Failure == nil || result.Failure.Kind != balance.FailureInvalidResponse {
			t.Fatalf("payload %#v result = %#v", response, result)
		}
	}
}

func TestKimiNormalizationPrefersValidNestedQuota(t *testing.T) {
	var response kimiUsageResp
	if err := json.Unmarshal([]byte(`{"usage":{"resetAt":"metadata only"},"data":{"usage":{"limit":100,"remaining":80}}}`), &response); err != nil {
		t.Fatal(err)
	}
	normalized := normalizeKimiUsage(response)
	if limit, ok := firstNumber(normalized.Usage, "limit"); !ok || limit != 100 {
		t.Fatalf("normalized response = %#v", normalized)
	}
}

func TestKimiBalanceRejectsMissingBalanceFields(t *testing.T) {
	useTestHTTPClient(t, func(*http.Request) (*http.Response, error) {
		return jsonResponse(`{"code":0,"status":true,"data":{}}`), nil
	})
	result := (KimiAPI{}).Fetch("kimi", "key", "")
	if result.Failure == nil || result.Failure.Kind != balance.FailureInvalidResponse {
		t.Fatalf("result = %#v", result)
	}
}

func TestKimiBalancePreservesStringBusinessCode(t *testing.T) {
	useTestHTTPClient(t, func(*http.Request) (*http.Response, error) {
		return jsonResponse(`{"code":0,"status":false,"scode":"INVALID_API_KEY"}`), nil
	})
	result := (KimiAPI{}).Fetch("kimi", "key", "")
	if result.Failure == nil || result.Failure.Kind != balance.FailureAuthentication || result.Failure.ProviderCode != "INVALID_API_KEY" {
		t.Fatalf("result = %#v", result)
	}
}

func TestBusinessErrorRedactsLongCredentialBeforeTruncation(t *testing.T) {
	key := "sk-" + strings.Repeat("business-secret", 30)
	useTestHTTPClient(t, func(*http.Request) (*http.Response, error) {
		return jsonResponse(`{"code":1,"status":false,"message":"invalid key ` + key + `"}`), nil
	})
	result := (KimiAPI{}).Fetch("kimi", key, "")
	if result.Error == "" || strings.Contains(result.Error, key[:120]) || !strings.Contains(result.Error, "[已隐藏]") {
		t.Fatalf("business error was not safely redacted: %#v", result)
	}
}

func TestNewAPILegacyRequiresResponseFields(t *testing.T) {
	tests := []struct {
		name         string
		subscription string
		usage        string
	}{
		{name: "missing subscription limits", subscription: `{}`},
		{name: "missing usage", subscription: `{"hard_limit_usd":10}`, usage: `{}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			useTestHTTPClient(t, func(request *http.Request) (*http.Response, error) {
				switch request.URL.Path {
				case "/api/usage/token/":
					return &http.Response{StatusCode: http.StatusNotFound, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"message":"not found"}`))}, nil
				case "/v1/dashboard/billing/subscription":
					return jsonResponse(test.subscription), nil
				case "/v1/dashboard/billing/usage":
					return jsonResponse(test.usage), nil
				default:
					t.Fatalf("unexpected path %q", request.URL.Path)
					return nil, nil
				}
			})
			result := (NewAPI{BaseURL: "https://new-api.example/v1"}).Fetch("new", "key", "")
			if result.Failure == nil || result.Failure.Kind != balance.FailureInvalidResponse {
				t.Fatalf("result = %#v", result)
			}
		})
	}
}

func TestSub2APITopLevelErrorAndUnknownQuota(t *testing.T) {
	errorResult := parseSub2APIUsage("sub", map[string]any{
		"code":    "INVALID_API_KEY",
		"message": "Invalid API key",
	})
	if errorResult.Failure == nil || errorResult.Failure.Kind != balance.FailureAuthentication || errorResult.Failure.ProviderCode != "INVALID_API_KEY" {
		t.Fatalf("top-level error = %#v", errorResult)
	}
	successResult := parseSub2APIUsage("sub", map[string]any{
		"code":  float64(0),
		"quota": map[string]any{"limit": float64(10), "remaining": float64(8)},
	})
	if successResult.Error != "" || len(successResult.QuotaWindows) != 1 {
		t.Fatalf("success envelope = %#v", successResult)
	}

	for _, values := range []map[string]any{
		{},
		{"limit": float64(10)},
		{"used": float64(2)},
	} {
		window := quotaWindowFromMap("速率限制", "5 小时额度", values, "USD")
		if !window.Unknown || window.CapacityPercent != 0 || window.RemainingPercent != 0 {
			t.Fatalf("values %#v produced %#v", values, window)
		}
	}
}

func TestSub2APINegativeSubscriptionLimitIsUnknown(t *testing.T) {
	result := parseSub2APIUsage("sub", map[string]any{
		"subscription": map[string]any{
			"daily_limit_usd": float64(-1),
			"daily_usage_usd": float64(0),
		},
	})
	if len(result.QuotaWindows) != 1 || !result.QuotaWindows[0].Unknown || result.QuotaWindows[0].Unlimited {
		t.Fatalf("result = %#v", result)
	}
}

func TestSub2APISubscriptionUsageIsClamped(t *testing.T) {
	result := parseSub2APIUsage("sub", map[string]any{
		"subscription": map[string]any{
			"daily_limit_usd":  float64(10),
			"daily_usage_usd":  float64(-3),
			"weekly_limit_usd": float64(10),
			"weekly_usage_usd": float64(20),
		},
	})
	if len(result.QuotaWindows) != 2 {
		t.Fatalf("windows = %#v", result.QuotaWindows)
	}
	if daily := result.QuotaWindows[0]; daily.Used != 0 || daily.Remaining != 10 {
		t.Fatalf("daily window = %#v", daily)
	}
	if weekly := result.QuotaWindows[1]; weekly.Used != 10 || weekly.Remaining != 0 {
		t.Fatalf("weekly window = %#v", weekly)
	}
}

func TestSub2APIDoesNotRetryAccountRestricted401(t *testing.T) {
	requests := 0
	useTestHTTPClient(t, func(*http.Request) (*http.Response, error) {
		requests++
		return &http.Response{
			StatusCode: http.StatusUnauthorized,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"code":"API_KEY_DISABLED","message":"API key is disabled"}`)),
		}, nil
	})
	result := (Sub2API{BaseURL: "https://sub2api.example/v1"}).Fetch("sub", "key", "")
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
	if result.Failure == nil || result.Failure.Kind != balance.FailureAccount {
		t.Fatalf("result = %#v", result)
	}
}

func TestGLMEmptyKeyDoesNotSendRequests(t *testing.T) {
	requests := 0
	useTestHTTPClient(t, func(*http.Request) (*http.Response, error) {
		requests++
		return jsonResponse(`{}`), nil
	})
	result := (GLMZai{}).Fetch("glm", "  ", "")
	if requests != 0 {
		t.Fatalf("requests = %d, want 0", requests)
	}
	if result.Failure == nil || result.Failure.Kind != balance.FailureAuthentication {
		t.Fatalf("result = %#v", result)
	}
}

func TestGLMPartialDetailFailuresBecomeTypedWarnings(t *testing.T) {
	useTestHTTPClient(t, func(request *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(request.URL.Path, "/quota/limit"):
			return jsonResponse(`{"code":200,"success":true,"data":{"limits":[{"type":"TOKENS_LIMIT","percentage":20,"unit":3,"number":5}]}}`), nil
		case strings.HasSuffix(request.URL.Path, "/model-usage"):
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     http.Header{"Retry-After": []string{"9"}},
				Body:       io.NopCloser(strings.NewReader(`{"code":"RATE_LIMITED","message":"slow down"}`)),
			}, nil
		case strings.HasSuffix(request.URL.Path, "/tool-usage"):
			return jsonResponse(`{"code":1004,"success":false,"message":"invalid api key"}`), nil
		default:
			t.Fatalf("unexpected GLM path %q", request.URL.Path)
			return nil, nil
		}
	})

	result := fetchGLMQuota("glm", "key", "", "https://api.z.ai", "GLM")
	if result.Error != "" || len(result.QuotaWindows) == 0 {
		t.Fatalf("partial GLM result = %#v", result)
	}
	if len(result.Warnings) != 2 {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
	warnings := map[string]balance.FailureInfo{}
	for _, warning := range result.Warnings {
		warnings[warning.Kind] = warning
	}
	if warning := warnings[balance.FailureRateLimited]; warning.HTTPStatus != http.StatusTooManyRequests || warning.RetryAfterSeconds != 9 || !strings.Contains(warning.Title, "模型统计") {
		t.Fatalf("model warning = %#v", warning)
	}
	if warning := warnings[balance.FailureAuthentication]; warning.ProviderCode != "1004" || !strings.Contains(warning.Title, "工具统计") {
		t.Fatalf("tool warning = %#v", warning)
	}
}

func TestGLMExplicitFailureCodeOverridesSuccessFlag(t *testing.T) {
	response := glmDetailResp{Code: float64(1004), Success: true, Message: "invalid api key"}
	if glmDetailSuccess(response) {
		t.Fatal("explicit GLM failure code was accepted because success=true")
	}
	failure := glmDetailFailure("模型统计", response, nil)
	result := errResult("glm", "GLM", failure)
	if result.Failure == nil || result.Failure.Kind != balance.FailureAuthentication || result.Failure.ProviderCode != "1004" {
		t.Fatalf("failure = %#v", result)
	}
}

func TestGLMExplicitFalseSuccessWithoutCodeIsBusinessFailure(t *testing.T) {
	var response glmQuotaResp
	if err := json.Unmarshal([]byte(`{"success":false,"message":"permission denied","data":{"limits":[{"type":"TOKENS_LIMIT","percentage":10}]}}`), &response); err != nil {
		t.Fatal(err)
	}
	failure := glmQuotaBusinessFailure(response, "key")
	result := errResult("glm", "GLM", failure)
	if result.Failure == nil || result.Failure.Kind != balance.FailurePermission {
		t.Fatalf("result = %#v", result)
	}
}

func TestNumericHelpersRejectNonFiniteFractionalAndOverflow(t *testing.T) {
	for _, value := range []any{float32(math.Inf(1)), jsonNumber("NaN"), "-Inf"} {
		if parsed, ok := numberValue(value); ok {
			t.Fatalf("numberValue(%#v) = %v, want rejection", value, parsed)
		}
	}
	for _, value := range []any{1.5, "9223372036854775808", math.Exp2(63)} {
		if parsed, ok := int64Value(value); ok {
			t.Fatalf("int64Value(%#v) = %d, want rejection", value, parsed)
		}
	}
	if parsed, ok := int64Value(int64(math.MaxInt64)); !ok || parsed != math.MaxInt64 {
		t.Fatalf("MaxInt64 = %d, %v", parsed, ok)
	}
	if parsed, ok := int64Value("-9223372036854775808"); !ok || parsed != math.MinInt64 {
		t.Fatalf("MinInt64 = %d, %v", parsed, ok)
	}
	window := kimiQuotaWindowBase(map[string]any{"resetIn": math.Exp2(63)}, "额度")
	if window.ResetInSeconds != 0 {
		t.Fatalf("overflow reset seconds = %d", window.ResetInSeconds)
	}
	result := balance.Result{}
	applyPrimaryWindow(&result, balance.QuotaWindow{Label: "额度", Total: math.Exp2(63), Used: 1, Remaining: math.Exp2(63) - 1})
	if result.TokensTotal != 0 || result.TokensUsed != 0 || result.TokensRemaining != 0 {
		t.Fatalf("overflow legacy token fields = %#v", result)
	}
}

func TestMoneyParsersRejectNonFiniteValues(t *testing.T) {
	t.Run("DeepSeek", func(t *testing.T) {
		useTestHTTPClient(t, func(*http.Request) (*http.Response, error) {
			return jsonResponse(`{"is_available":true,"balance_infos":[{"currency":"USD","total_balance":"NaN"}]}`), nil
		})
		result := (DeepSeek{}).Fetch("deepseek", "key", "")
		if result.Failure == nil || result.Failure.Kind != balance.FailureInvalidResponse {
			t.Fatalf("result = %#v", result)
		}
	})
	t.Run("Claude cost", func(t *testing.T) {
		useTestHTTPClient(t, func(*http.Request) (*http.Response, error) {
			return jsonResponse(`{"data":[{"results":[{"amount":"Infinity","currency":"USD"}]}],"has_more":false}`), nil
		})
		_, _, err := fetchClaudeCosts("https://api.anthropic.com/v1/organizations/cost_report", url.Values{}, "", nil)
		var providerErr *ProviderError
		if !errors.As(err, &providerErr) || providerErr.Kind != balance.FailureInvalidResponse {
			t.Fatalf("error = %#v", err)
		}
	})
}

func TestClaudeUsageRejectsCombinedTokenOverflow(t *testing.T) {
	useTestHTTPClient(t, func(*http.Request) (*http.Response, error) {
		return jsonResponse(`{"data":[{"results":[{"uncached_input_tokens":9223372036854775807,"output_tokens":1}]}],"has_more":false}`), nil
	})
	_, err := fetchClaudeUsage("https://api.anthropic.com/v1/organizations/usage_report/messages", url.Values{}, "", nil)
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || providerErr.Kind != balance.FailureInvalidResponse {
		t.Fatalf("overflow error = %#v", err)
	}
}

func TestNewAPIRejectsOverflowAfterQuotaConversion(t *testing.T) {
	response := newAPITokenUsageResp{HasQuotaFields: true}
	response.Data.TotalGranted = math.MaxFloat64
	response.Data.TotalUsed = math.MaxFloat64
	response.Data.TotalAvailable = math.MaxFloat64
	status := newAPIStatusResp{Success: true}
	status.Data.QuotaPerUnit = math.SmallestNonzeroFloat64
	result := parseNewAPITokenUsage("new", response, status)
	if result.Failure == nil || result.Failure.Kind != balance.FailureInvalidResponse {
		t.Fatalf("result = %#v", result)
	}
}

func TestNewAPIUnlimitedUsageTracksUsedFieldWithoutOtherQuotaFields(t *testing.T) {
	var response newAPITokenUsageResp
	if err := json.Unmarshal([]byte(`{"code":true,"data":{"unlimited_quota":true,"total_used":42}}`), &response); err != nil {
		t.Fatal(err)
	}
	if !response.HasQuotaFields || !response.HasUsedField {
		t.Fatalf("unlimited response field presence = %#v", response)
	}
	result := parseNewAPITokenUsage("new", response, newAPIStatusResp{})
	if len(result.QuotaWindows) != 1 || !result.QuotaWindows[0].Unlimited || !result.QuotaWindows[0].ShowUsedWhenUnlimited {
		t.Fatalf("unlimited result = %#v", result)
	}
}

func TestMiniMaxIntegerFieldsRejectFractionalAndOutOfRangeValues(t *testing.T) {
	for _, value := range []any{1.5, math.Exp2(40), "not-a-number"} {
		if parsed, ok := miniMaxInt(map[string]any{"status": value}, "status"); ok {
			t.Fatalf("miniMaxInt(%#v) = %d, want rejection", value, parsed)
		}
	}
	if parsed, ok := miniMaxInt(map[string]any{"status": "1004"}, "status"); !ok || parsed != 1004 {
		t.Fatalf("miniMaxInt(valid) = %d, %v", parsed, ok)
	}
}

func TestSub2QuotaUsesConservativeRemainingWhenFieldsConflict(t *testing.T) {
	window := quotaWindowFromMap("额度", "总额度", map[string]any{
		"total": float64(100), "used": float64(90), "remaining": float64(90),
	}, "USD")
	if window.Used != 90 || window.Remaining != 10 || window.RemainingPercent != 10 || !strings.Contains(window.Status, "不一致") {
		t.Fatalf("conflicting Sub2 quota = %#v", window)
	}
}

func TestNewAPILegacyStringErrorIsTyped(t *testing.T) {
	var response newAPISubscriptionResp
	if err := json.Unmarshal([]byte(`{"error":"invalid api key"}`), &response); err != nil {
		t.Fatal(err)
	}
	result := errResult("new", "New API", response.Error.providerError("fallback"))
	if result.Failure == nil || result.Failure.Kind != balance.FailureAuthentication {
		t.Fatalf("result = %#v", result)
	}
}

func TestNormalizeTimestampProducesRFC3339WithTimezone(t *testing.T) {
	if got := normalizeTimestamp("2026-07-29T10:00:00+08:00"); got != "2026-07-29T02:00:00Z" {
		t.Fatalf("offset timestamp = %q", got)
	}
	const wantUTC = "2026-07-29T10:00:00Z"
	if got := normalizeTimestamp("2026-07-29 10:00:00"); got != wantUTC {
		t.Fatalf("timezone-less timestamp = %q, want %q", got, wantUTC)
	}
	window, ok := kimiQuotaWindow(map[string]any{
		"limit":     float64(10),
		"used":      float64(2),
		"resetTime": "2026-07-29 10:00:00",
	}, "5 小时配额")
	if !ok || window.ResetAt != wantUTC {
		t.Fatalf("Kimi reset window = %#v", window)
	}
	if got := normalizedTimestampForDisplay("next billing cycle"); got != "next billing cycle" {
		t.Fatalf("unparseable display timestamp = %q", got)
	}
	if got := normalizeTimestamp(int64(math.MaxInt64)); got != "" {
		t.Fatalf("out-of-range timestamp = %q", got)
	}
	if got := formatUnixTimestamp(int64(math.MaxInt64)); got != "" {
		t.Fatalf("out-of-range formatted timestamp = %q", got)
	}
}

func TestRetryAfterHTTPDateRoundsUp(t *testing.T) {
	now := time.Date(2026, 7, 29, 0, 0, 0, 500_000_000, time.UTC)
	retryAt := now.Add(1500 * time.Millisecond).Format(http.TimeFormat)
	if got := parseRetryAfter(retryAt, now); got != 2 {
		t.Fatalf("Retry-After = %d, want 2", got)
	}
}

type closeTrackingTransport struct {
	closed int
}

func (*closeTrackingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("not used")
}

func (transport *closeTrackingTransport) CloseIdleConnections() {
	transport.closed++
}

func TestCloseIdleConnectionsClosesConfiguredClient(t *testing.T) {
	original := httpClient
	transport := &closeTrackingTransport{}
	httpClient = &http.Client{Transport: transport}
	t.Cleanup(func() { httpClient = original })

	CloseIdleConnections()
	if transport.closed != 1 {
		t.Fatalf("CloseIdleConnections calls = %d, want 1", transport.closed)
	}
}
