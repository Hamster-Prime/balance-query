package main

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Hamster-Prime/balance-query/internal/balance"
)

func TestApplyRuntimeConfigParsesYAMLAndNormalizesValues(t *testing.T) {
	previousState := snapshotState()
	t.Cleanup(func() {
		stateMu.Lock()
		state = previousState
		stateMu.Unlock()
		resultCache.SetTTL(time.Duration(previousState.CacheTTLSeconds) * time.Second)
		resultCache.Flush()
	})

	resultCache.Set("stale-entry", balance.Result{Provider: "旧缓存"})
	raw, err := json.Marshal(lifecycleRequest{ConfigYAML: []byte(`
cache_ttl_seconds: 3
provider_mappings:
  provider-one: sub2api
  provider-two: newapi
  invalid-provider: made_up
`)})
	if err != nil {
		t.Fatalf("marshal lifecycle request: %v", err)
	}

	if err := applyRuntimeConfig(raw); err != nil {
		t.Fatalf("applyRuntimeConfig() error = %v", err)
	}

	got := snapshotState()
	if got.CacheTTLSeconds != 10 {
		t.Fatalf("CacheTTLSeconds = %d, want minimum 10", got.CacheTTLSeconds)
	}
	if got.ProviderMappings["provider-one"] != balance.ProviderSub2API {
		t.Fatalf("provider-one mapping = %q, want %q", got.ProviderMappings["provider-one"], balance.ProviderSub2API)
	}
	if got.ProviderMappings["provider-two"] != balance.ProviderNewAPI {
		t.Fatalf("provider-two mapping = %q, want %q", got.ProviderMappings["provider-two"], balance.ProviderNewAPI)
	}
	if _, exists := got.ProviderMappings["invalid-provider"]; exists {
		t.Fatal("unknown provider mapping was not removed")
	}
	if _, exists := resultCache.Get("stale-entry"); exists {
		t.Fatal("changing the TTL did not flush existing cache entries")
	}
}

func TestApplyRuntimeConfigTTLBounds(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{name: "cache disabled", in: 0, want: 0},
		{name: "minimum", in: -1, want: 10},
		{name: "unchanged", in: 300, want: 300},
		{name: "maximum", in: 100000, want: 86400},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := normalizeTTL(test.in); got != test.want {
				t.Fatalf("normalizeTTL(%d) = %d, want %d", test.in, got, test.want)
			}
		})
	}
}

func TestApplyRuntimeConfigDefaultsTTLWhenOmitted(t *testing.T) {
	previousState := snapshotState()
	t.Cleanup(func() {
		stateMu.Lock()
		state = previousState
		stateMu.Unlock()
		resultCache.SetTTL(time.Duration(previousState.CacheTTLSeconds) * time.Second)
		resultCache.Flush()
	})

	raw, err := json.Marshal(lifecycleRequest{ConfigYAML: []byte(`
provider_mappings:
  provider-one: deepseek
`)})
	if err != nil {
		t.Fatalf("marshal lifecycle request: %v", err)
	}
	if err := applyRuntimeConfig(raw); err != nil {
		t.Fatalf("applyRuntimeConfig() error = %v", err)
	}
	if got := currentTTL(); got != defaultTTLSeconds {
		t.Fatalf("TTL with omitted cache_ttl_seconds = %d, want default %d", got, defaultTTLSeconds)
	}
}

func TestManagementRegisterSeparatesAuthenticatedQueryAndResource(t *testing.T) {
	raw, err := handleMethod("management.register", nil)
	if err != nil {
		t.Fatalf("handleMethod(management.register) error = %v", err)
	}

	var registration managementRegistration
	decodeOKEnvelope(t, raw, &registration)

	if len(registration.Routes) != 1 {
		t.Fatalf("management routes = %d, want 1: %#v", len(registration.Routes), registration.Routes)
	}
	if route := registration.Routes[0]; route.Method != http.MethodPost || route.Path != queryPath {
		t.Fatalf("management route = %#v, want POST %s", route, queryPath)
	}
	if len(registration.Resources) != 1 {
		t.Fatalf("resource routes = %d, want 1: %#v", len(registration.Resources), registration.Resources)
	}
	if resource := registration.Resources[0]; resource.Path != resourcePath || strings.TrimSpace(resource.Menu) == "" {
		t.Fatalf("resource route = %#v, want menu resource %s", resource, resourcePath)
	}

	// CPA authenticates declarations in Routes and deliberately serves Resources
	// without management authentication. Keep the secret-bearing query endpoint out
	// of Resources and the navigable dashboard out of authenticated API routes.
	for _, resource := range registration.Resources {
		if resource.Path == queryPath {
			t.Fatalf("secret-bearing query path %q was exposed as an unauthenticated resource", queryPath)
		}
	}
	for _, route := range registration.Routes {
		if route.Path == resourcePath {
			t.Fatalf("dashboard path %q was incorrectly registered as a management API route", resourcePath)
		}
	}
}

func TestValidateAccountQuery(t *testing.T) {
	valid := accountQuery{
		ID:          "provider-id",
		ProviderKey: "provider-key",
		AccountName: "生产环境",
		BaseURL:     "https://relay.example.com/v1",
		APIKey:      "sk-secret",
		QueryType:   balance.ProviderSub2API,
	}

	tests := []struct {
		name      string
		mutate    func(*accountQuery)
		wantError string
	}{
		{name: "valid", mutate: func(*accountQuery) {}},
		{name: "missing id", mutate: func(q *accountQuery) { q.ID = " " }, wantError: "账户标识"},
		{name: "missing provider key", mutate: func(q *accountQuery) { q.ProviderKey = "" }, wantError: "提供商标识"},
		{name: "missing name", mutate: func(q *accountQuery) { q.AccountName = "" }, wantError: "提供商名称"},
		{name: "missing api key", mutate: func(q *accountQuery) { q.APIKey = "\t" }, wantError: "接口密钥"},
		{name: "unknown query type", mutate: func(q *accountQuery) { q.QueryType = "unknown" }, wantError: "未知"},
		{name: "missing relay URL", mutate: func(q *accountQuery) { q.BaseURL = "" }, wantError: "HTTP(S) URL"},
		{name: "non HTTP relay URL", mutate: func(q *accountQuery) { q.BaseURL = "file:///tmp/api" }, wantError: "HTTP(S) URL"},
		{name: "URL with credentials", mutate: func(q *accountQuery) { q.BaseURL = "https://user:pass@relay.example.com/v1" }, wantError: "HTTP(S) URL"},
		{name: "invalid proxy URL", mutate: func(q *accountQuery) { q.ProxyURL = "file:///tmp/proxy" }, wantError: "代理地址"},
		{name: "proxy URL without host", mutate: func(q *accountQuery) { q.ProxyURL = "socks5:///proxy" }, wantError: "代理地址"},
		{name: "explicit direct proxy", mutate: func(q *accountQuery) { q.ProxyURL = "direct" }},
		{name: "explicit none proxy", mutate: func(q *accountQuery) { q.ProxyURL = "none" }},
		{
			name: "official provider also requires source URL",
			mutate: func(q *accountQuery) {
				q.QueryType = balance.ProviderDeepSeek
				q.BaseURL = ""
			},
			wantError: "HTTP(S) URL",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query := valid
			test.mutate(&query)
			err := validateAccountQuery(query)
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("validateAccountQuery() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("validateAccountQuery() error = %v, want containing %q", err, test.wantError)
			}
		})
	}
}

func TestHandleManagementRequestValidationEnvelope(t *testing.T) {
	tests := []struct {
		name       string
		request    managementRequest
		wantStatus int
		wantError  string
	}{
		{
			name:       "query only accepts post",
			request:    managementRequest{Method: http.MethodGet, Path: queryPath},
			wantStatus: http.StatusMethodNotAllowed,
			wantError:  "仅支持 POST",
		},
		{
			name:       "malformed body",
			request:    managementRequest{Method: http.MethodPost, Path: queryPath, Body: []byte("{")},
			wantStatus: http.StatusBadRequest,
			wantError:  "格式不正确",
		},
		{
			name: "invalid account",
			request: managementRequest{
				Method: http.MethodPost,
				Path:   queryPath,
				Body:   []byte(`{"accounts":[{"id":"one","provider_key":"provider-one","account_name":"测试","base_url":"https://relay.example.com/v1","query_type":"newapi"}]}`),
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "接口密钥",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rawRequest, err := json.Marshal(test.request)
			if err != nil {
				t.Fatalf("marshal management request: %v", err)
			}
			rawResponse, err := handleManagementRequest(rawRequest)
			if err != nil {
				t.Fatalf("handleManagementRequest() error = %v", err)
			}

			var response managementResponse
			decodeOKEnvelope(t, rawResponse, &response)
			if response.StatusCode != test.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", response.StatusCode, test.wantStatus, response.Body)
			}
			var body map[string]string
			if err := json.Unmarshal(response.Body, &body); err != nil {
				t.Fatalf("decode response body: %v; body=%s", err, response.Body)
			}
			if !strings.Contains(body["error"], test.wantError) {
				t.Fatalf("error = %q, want containing %q", body["error"], test.wantError)
			}
		})
	}
}

func TestHandleMethodUnknownMethodUsesErrorEnvelope(t *testing.T) {
	raw, err := handleMethod("not.a.real.method", nil)
	if err != nil {
		t.Fatalf("handleMethod() error = %v", err)
	}
	var got envelope
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if got.OK || got.Error == nil || got.Error.Code != "unknown_method" {
		t.Fatalf("envelope = %#v, want unknown_method error", got)
	}
}

func TestAccountCacheKeyIsStableAndDoesNotExposeSecrets(t *testing.T) {
	account := accountQuery{
		ID:          "account-1",
		ProviderKey: "provider-1",
		AccountName: "生产环境",
		BaseURL:     " https://relay.example.com/v1 ",
		APIKey:      "sk-very-secret-value",
		QueryType:   balance.ProviderNewAPI,
	}

	first := accountCacheKey(account)
	second := accountCacheKey(account)
	if first != second {
		t.Fatalf("cache key is unstable: %q != %q", first, second)
	}
	if len(first) != 64 {
		t.Fatalf("cache key length = %d, want 64", len(first))
	}
	if _, err := hex.DecodeString(first); err != nil {
		t.Fatalf("cache key is not hex: %q: %v", first, err)
	}
	for _, secret := range []string{account.APIKey, "very-secret"} {
		if strings.Contains(first, secret) {
			t.Fatalf("cache key %q leaked credential material %q", first, secret)
		}
	}

	displayOnlyChange := account
	displayOnlyChange.AccountName = "另一个显示名称"
	if got := accountCacheKey(displayOnlyChange); got != first {
		t.Fatalf("display-only fields changed cache identity: got %q, want %q", got, first)
	}

	changedSecret := account
	changedSecret.APIKey = "sk-another-secret"
	if got := accountCacheKey(changedSecret); got == first {
		t.Fatal("different API keys produced the same cache key")
	}

	changedProxy := account
	changedProxy.ProxyURL = "socks5://127.0.0.1:1080"
	if got := accountCacheKey(changedProxy); got == first {
		t.Fatal("different proxy settings produced the same cache key")
	}
}

func TestRedactResultSecret(t *testing.T) {
	const secret = "sk-very-secret-value"
	result := balance.Result{
		Error:        "上游拒绝 " + secret,
		QuotaDisplay: "账户 " + secret,
		Plan:         secret,
		ResetAt:      "稍后 " + secret,
		Extra:        map[string]string{"响应": "包含 " + secret},
	}
	redactResultSecret(&result, secret)
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal redacted result: %v", err)
	}
	if strings.Contains(string(raw), secret) {
		t.Fatalf("redacted result still contains secret: %s", raw)
	}
	if !strings.Contains(string(raw), maskAPIKey(secret)) {
		t.Fatalf("redacted result does not contain masked key: %s", raw)
	}
}

func TestLocalizeFetchError(t *testing.T) {
	tests := map[string]string{
		`余额接口返回 HTTP 401：{"error":"invalid api key"}`:           "余额接口拒绝了密钥",
		"Get https://example.com: dial tcp: connection refused": "无法连接余额服务",
		"upstream returned an unexpected response":              "余额查询未成功",
	}
	for input, want := range tests {
		got := localizeFetchError(input)
		if !strings.Contains(got, want) {
			t.Fatalf("localizeFetchError(%q) = %q, want containing %q", input, got, want)
		}
		if got == input {
			t.Fatalf("localized error still exposes raw upstream message: %q", got)
		}
	}
}

func snapshotState() balance.PluginConfig {
	stateMu.RLock()
	defer stateMu.RUnlock()
	mappings := make(map[string]balance.ProviderType, len(state.ProviderMappings))
	for key, value := range state.ProviderMappings {
		mappings[key] = value
	}
	return balance.PluginConfig{
		CacheTTLSeconds:  state.CacheTTLSeconds,
		ProviderMappings: mappings,
	}
}

func decodeOKEnvelope(t *testing.T, raw []byte, target any) {
	t.Helper()
	var wrapped envelope
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		t.Fatalf("decode envelope: %v; payload=%s", err, raw)
	}
	if !wrapped.OK || wrapped.Error != nil {
		t.Fatalf("envelope = %#v, want successful result", wrapped)
	}
	if err := json.Unmarshal(wrapped.Result, target); err != nil {
		t.Fatalf("decode envelope result: %v; result=%s", err, wrapped.Result)
	}
}
