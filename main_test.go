package main

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
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

func TestApplyRuntimeConfigEmptyYAMLResetsState(t *testing.T) {
	previousState := snapshotState()
	t.Cleanup(func() {
		stateMu.Lock()
		state = previousState
		stateMu.Unlock()
		resultCache.SetTTL(time.Duration(previousState.CacheTTLSeconds) * time.Second)
		resultCache.Flush()
	})

	stateMu.Lock()
	state = balance.PluginConfig{
		CacheTTLSeconds:  900,
		ProviderMappings: map[string]balance.ProviderType{"stale": balance.ProviderNewAPI},
	}
	stateMu.Unlock()
	resultCache.SetTTL(900 * time.Second)
	resultCache.Set("stale", balance.Result{Provider: "stale"})

	raw, err := json.Marshal(lifecycleRequest{ConfigYAML: []byte(" \n\t")})
	if err != nil {
		t.Fatal(err)
	}
	if err := applyRuntimeConfig(raw); err != nil {
		t.Fatalf("applyRuntimeConfig() error = %v", err)
	}
	got := snapshotState()
	if got.CacheTTLSeconds != defaultTTLSeconds || len(got.ProviderMappings) != 0 {
		t.Fatalf("state after empty config = %#v", got)
	}
	if _, ok := resultCache.Get("stale"); ok {
		t.Fatal("empty config did not flush stale cache")
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
			name: "official provider can use its default URL",
			mutate: func(q *accountQuery) {
				q.QueryType = balance.ProviderDeepSeek
				q.BaseURL = ""
			},
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

func TestResolveConfiguredQueryTypeRejectsStalePage(t *testing.T) {
	previousState := snapshotState()
	t.Cleanup(func() {
		stateMu.Lock()
		state = previousState
		stateMu.Unlock()
	})
	stateMu.Lock()
	state.ProviderMappings = map[string]balance.ProviderType{
		"legacy-key": balance.ProviderDeepSeek,
	}
	stateMu.Unlock()

	matching := accountQuery{
		ProviderKey: "new-display-key",
		MappingKey:  "legacy-key",
		QueryType:   balance.ProviderDeepSeek,
	}
	if err := resolveConfiguredQueryType(&matching); err != nil {
		t.Fatalf("resolveConfiguredQueryType() error = %v", err)
	}
	if matching.QueryType != balance.ProviderDeepSeek || matching.MappingKey != "legacy-key" {
		t.Fatalf("resolved account = %#v", matching)
	}

	stale := matching
	stale.QueryType = balance.ProviderNewAPI
	if err := resolveConfiguredQueryType(&stale); err == nil || !strings.Contains(err.Error(), "其他页面修改") {
		t.Fatalf("stale mapping error = %v", err)
	}

	missing := matching
	missing.MappingKey = "unknown"
	if err := resolveConfiguredQueryType(&missing); err == nil || !strings.Contains(err.Error(), "没有此提供商映射") {
		t.Fatalf("missing mapping error = %v", err)
	}
}

func TestFetchAccountsPreservesProviderKeyOnCacheHit(t *testing.T) {
	previousState := snapshotState()
	t.Cleanup(func() {
		stateMu.Lock()
		state = previousState
		stateMu.Unlock()
		resultCache.SetTTL(time.Duration(previousState.CacheTTLSeconds) * time.Second)
		resultCache.Flush()
	})

	account := accountQuery{
		ID:          "account-1",
		ProviderKey: "OpenAI%20%E5%85%BC%E5%AE%B9|relay|https%3A%2F%2Frelay.example.com%2Fv1",
		AccountName: "Relay · 密钥 1",
		BaseURL:     "https://relay.example.com/v1",
		APIKey:      "sk-provider-key-test",
		QueryType:   balance.ProviderNewAPI,
	}
	resultCache.SetTTL(time.Hour)
	resultCache.Set(accountCacheKey(account), balance.Result{Provider: "New API", FetchedAt: time.Now()})

	results := fetchAccounts([]accountQuery{account}, false)
	if len(results) != 1 {
		t.Fatalf("fetchAccounts() returned %d results, want 1", len(results))
	}
	if results[0].ProviderKey != account.ProviderKey {
		t.Fatalf("ProviderKey = %q, want %q", results[0].ProviderKey, account.ProviderKey)
	}
	if results[0].AccountName != account.AccountName || results[0].BaseURL != account.BaseURL {
		t.Fatalf("cached display metadata was not refreshed: %#v", results[0])
	}
}

func TestFetchAccountsCoalescesConcurrentIdenticalUpstreamQueries(t *testing.T) {
	cleanupCacheForTest(t, time.Hour)
	var usageRequests atomic.Int32
	useFetcherForTest(t, fetcherFunc(func(authID, _, _ string) balance.Result {
		usageRequests.Add(1)
		time.Sleep(40 * time.Millisecond)
		return balance.Result{Provider: "New API", AuthID: authID, QuotaDisplay: "可用 80 USD", FetchedAt: time.Now()}
	}))

	first := testNewAPIAccount("https://new-api.example/v1")
	second := first
	second.ID = "another-auth-id"
	second.ProviderKey = "another-display-provider"
	second.AccountName = "另一个显示名称"

	var wait sync.WaitGroup
	results := make([][]balance.Result, 2)
	for index, account := range []accountQuery{first, second} {
		wait.Add(1)
		go func(i int, item accountQuery) {
			defer wait.Done()
			results[i] = fetchAccounts([]accountQuery{item}, false)
		}(index, account)
	}
	wait.Wait()
	if got := usageRequests.Load(); got != 1 {
		t.Fatalf("usage endpoint requests = %d, want 1", got)
	}
	for index, result := range results {
		if len(result) != 1 || result[0].Error != "" {
			t.Fatalf("result %d = %#v", index, result)
		}
	}
	if results[1][0].ProviderKey != second.ProviderKey || results[1][0].AuthID != second.ID {
		t.Fatalf("coalesced result kept another account's display metadata: %#v", results[1][0])
	}
}

func TestRefreshFailurePreservesLastSuccessfulCache(t *testing.T) {
	cleanupCacheForTest(t, time.Hour)
	useFetcherForTest(t, fetcherFunc(func(authID, _, _ string) balance.Result {
		return balance.Result{Provider: "New API", AuthID: authID, Error: "余额接口返回 HTTP 503", FetchedAt: time.Now()}
	}))
	account := testNewAPIAccount("https://new-api.example/v1")
	cached := balance.Result{Provider: "New API", QuotaDisplay: "上次成功", FetchedAt: time.Now()}
	resultCache.Set(accountCacheKey(account), cached)

	refreshed := fetchAccounts([]accountQuery{account}, true)
	if len(refreshed) != 1 || refreshed[0].Error == "" {
		t.Fatalf("refresh result = %#v, want current failure", refreshed)
	}
	stillCached, ok := resultCache.Get(accountCacheKey(account))
	if !ok || stillCached.Error != "" || stillCached.QuotaDisplay != "上次成功" {
		t.Fatalf("successful cache was replaced by refresh failure: %#v, ok=%v", stillCached, ok)
	}
}

func TestFailureResultUsesShortNegativeCache(t *testing.T) {
	cleanupCacheForTest(t, time.Hour)
	var requests atomic.Int32
	useFetcherForTest(t, fetcherFunc(func(authID, _, _ string) balance.Result {
		requests.Add(1)
		return balance.Result{Provider: "New API", AuthID: authID, Error: "余额接口返回 HTTP 401：invalid api key", FetchedAt: time.Now()}
	}))
	account := testNewAPIAccount("https://new-api.example/v1")

	first := fetchAccounts([]accountQuery{account}, false)
	second := fetchAccounts([]accountQuery{account}, false)
	if len(first) != 1 || first[0].Failure == nil || first[0].Failure.Kind != balance.FailureAuthentication {
		t.Fatalf("first failure = %#v", first)
	}
	if len(second) != 1 || second[0].Error == "" {
		t.Fatalf("cached failure = %#v", second)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("upstream requests = %d, want 1 due to negative cache", got)
	}
}

func TestPartialWarningUsesShortCache(t *testing.T) {
	cleanupCacheForTest(t, time.Hour)
	var requests atomic.Int32
	useFetcherForTest(t, fetcherFunc(func(authID, _, _ string) balance.Result {
		requests.Add(1)
		return balance.Result{
			Provider: "Claude", AuthID: authID, QuotaDisplay: "近 30 天使用 100 令牌",
			Warnings: []balance.FailureInfo{{
				Kind: balance.FailurePermission, Title: "费用查询 · 权限不足", Reason: "费用接口无权限",
			}},
			FetchedAt: time.Now(),
		}
	}))
	account := testNewAPIAccount("https://provider.example/v1")
	account.QueryType = balance.ProviderClaudeAdmin

	first := fetchAccounts([]accountQuery{account}, false)
	second := fetchAccounts([]accountQuery{account}, false)
	if len(first) != 1 || first[0].Error != "" || len(first[0].Warnings) != 1 {
		t.Fatalf("partial result = %#v", first)
	}
	if len(second) != 1 || len(second[0].Warnings) != 1 || requests.Load() != 1 {
		t.Fatalf("partial warning was not briefly cached: results=%#v requests=%d", second, requests.Load())
	}
}

func TestProviderPanicBecomesAccountFailure(t *testing.T) {
	cleanupCacheForTest(t, time.Hour)
	useFetcherForTest(t, fetcherFunc(func(string, string, string) balance.Result {
		panic("provider bug with secret")
	}))
	account := testNewAPIAccount("https://provider.example/v1")
	results := fetchAccounts([]accountQuery{account}, true)
	if len(results) != 1 || results[0].Error == "" || results[0].Failure == nil {
		t.Fatalf("panic result = %#v", results)
	}
	if strings.Contains(results[0].Error, "provider bug") {
		t.Fatalf("panic detail leaked to result: %#v", results[0])
	}
}

func TestFetchAccountRedactsSecretFromClientMetadata(t *testing.T) {
	const secret = "sk-client-metadata-secret"
	useFetcherForTest(t, fetcherFunc(func(authID, _, _ string) balance.Result {
		return balance.Result{
			Provider: "测试", AuthID: authID,
			Extra:     map[string]string{"上游 " + secret: "响应 " + secret},
			FetchedAt: time.Now(),
		}
	}))
	account := accountQuery{
		ID:          "auth-" + secret,
		ProviderKey: "provider-" + secret,
		AccountName: "账户 " + secret,
		BaseURL:     "https://provider.example/v1?token=" + secret,
		APIKey:      secret,
		QueryType:   balance.ProviderNewAPI,
	}
	result := fetchAccount(account)
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), secret) {
		t.Fatalf("fetch result leaked API key from client metadata: %s", raw)
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
	displayOnlyChange.ID = "renumbered-auth-id"
	displayOnlyChange.ProviderKey = "renamed-provider"
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
		Error:           "上游拒绝 " + secret,
		QuotaDisplay:    "账户 " + secret,
		Plan:            secret,
		ResetAt:         "稍后 " + secret,
		BalanceCurrency: secret,
		Extra:           map[string]string{"响应 " + secret: "包含 " + secret},
		Failure: &balance.FailureInfo{
			Kind: secret, Title: "失败 " + secret, Reason: secret,
			Suggestion: secret, ProviderCode: secret, RequestID: secret,
		},
		Warnings: []balance.FailureInfo{{
			Kind: secret, Title: "部分失败 " + secret, Reason: secret,
			Suggestion: secret, ProviderCode: secret, RequestID: secret,
		}},
		QuotaWindows: []balance.QuotaWindow{{
			Group: secret, Label: "周期 " + secret, Unit: secret,
			ResetAt: "稍后 " + secret, Status: "状态 " + secret,
			AggregationScope: secret, AggregationKey: "聚合 " + secret,
		}},
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

func TestDecorateAccountResultDoesNotMutateSharedResult(t *testing.T) {
	const secret = "sk-shared-secret"
	original := balance.Result{
		Failure:  &balance.FailureInfo{Reason: "失败 " + secret},
		Warnings: []balance.FailureInfo{{Reason: "警告 " + secret}},
		Extra:    map[string]string{"详情": "包含 " + secret},
		QuotaWindows: []balance.QuotaWindow{{
			Label: "周期 " + secret,
		}},
	}
	account := accountQuery{
		ID: "account", ProviderKey: "provider", AccountName: "账户",
		APIKey: secret,
	}

	first := decorateAccountResult(original, account)
	second := decorateAccountResult(original, account)
	if original.Failure.Reason != "失败 "+secret || original.Warnings[0].Reason != "警告 "+secret ||
		original.Extra["详情"] != "包含 "+secret || original.QuotaWindows[0].Label != "周期 "+secret {
		t.Fatalf("decorateAccountResult mutated its cached input: %#v", original)
	}

	first.Failure.Reason = "changed"
	first.Warnings[0].Reason = "changed"
	first.Extra["详情"] = "changed"
	first.QuotaWindows[0].Label = "changed"
	if second.Failure.Reason == "changed" || second.Warnings[0].Reason == "changed" ||
		second.Extra["详情"] == "changed" || second.QuotaWindows[0].Label == "changed" {
		t.Fatalf("decorated results still share mutable data: first=%#v second=%#v", first, second)
	}
}

func TestNormalizeAccountQueryTrimsTransportFields(t *testing.T) {
	account := accountQuery{
		ID: " account ", ProviderKey: " provider ", MappingKey: " mapping ",
		AccountName: " name ", BaseURL: " https://example.com/v1/ ",
		APIKey: " secret ", ProxyURL: " direct ", QueryType: " deepseek ",
	}
	normalizeAccountQuery(&account)
	if account.ID != "account" || account.ProviderKey != "provider" || account.MappingKey != "mapping" ||
		account.AccountName != "name" || account.BaseURL != "https://example.com/v1/" ||
		account.APIKey != "secret" || account.ProxyURL != "direct" || account.QueryType != balance.ProviderDeepSeek {
		t.Fatalf("normalizeAccountQuery() = %#v", account)
	}
}

func TestLocalizeFetchErrorPreservesAuditedConsoleOnlyReason(t *testing.T) {
	message := "小米官方未提供模型 API Key 的余额查询接口；按量余额只能在 MiMo 控制台登录后查看"
	if got := localizeFetchError(message); got != message {
		t.Fatalf("localizeFetchError() = %q, want audited reason %q", got, message)
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

func TestClassifyFetchFailureMatrix(t *testing.T) {
	tests := []struct {
		name         string
		provider     balance.ProviderType
		message      string
		existing     *balance.FailureInfo
		wantKind     string
		wantRetry    bool
		wantContains string
	}{
		{name: "payment required", message: "余额接口返回 HTTP 402", wantKind: balance.FailureInsufficientFund, wantContains: "余额不足"},
		{name: "dns beats json hostname", message: "请求余额接口失败：Get https://json.example: dial tcp: lookup json.example: no such host", wantKind: balance.FailureDNS, wantRetry: true},
		{name: "proxy only on explicit proxy failure", message: "请求余额接口失败：Get https://proxy.example: connection refused", wantKind: balance.FailureNetwork, wantRetry: true},
		{name: "kimi overload", provider: balance.ProviderKimiAPI, message: "HTTP 429", existing: &balance.FailureInfo{ProviderCode: "engine_overloaded_error"}, wantKind: balance.FailureService, wantRetry: true, wantContains: "过载"},
		{name: "kimi quota", provider: balance.ProviderKimiAPI, message: "HTTP 429", existing: &balance.FailureInfo{ProviderCode: "exceeded_current_quota_error"}, wantKind: balance.FailureQuotaExhausted, wantRetry: true, wantContains: "额度"},
		{name: "minimax five hour", provider: balance.ProviderMiniMaxCodingGlobal, message: "business failure", existing: &balance.FailureInfo{ProviderCode: "2056"}, wantKind: balance.FailureQuotaExhausted, wantRetry: true, wantContains: "5 小时"},
		{name: "glm key type", provider: balance.ProviderGLMZAI, message: "business failure", existing: &balance.FailureInfo{ProviderCode: "1315"}, wantKind: balance.FailureAuthentication, wantContains: "密钥类型"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := classifyFetchFailure(test.provider, test.message, test.existing)
			if got == nil || got.Kind != test.wantKind || got.Retryable != test.wantRetry {
				t.Fatalf("classifyFetchFailure() = %#v", got)
			}
			if test.wantContains != "" && !strings.Contains(got.Reason, test.wantContains) {
				t.Fatalf("reason = %q, want containing %q", got.Reason, test.wantContains)
			}
		})
	}
}

func cleanupCacheForTest(t *testing.T, ttl time.Duration) {
	t.Helper()
	previousState := snapshotState()
	resultCache.Flush()
	resultCache.SetTTL(ttl)
	stateMu.Lock()
	state.CacheTTLSeconds = int(ttl / time.Second)
	stateMu.Unlock()
	t.Cleanup(func() {
		resultCache.Flush()
		resultCache.SetTTL(time.Duration(previousState.CacheTTLSeconds) * time.Second)
		stateMu.Lock()
		state = previousState
		stateMu.Unlock()
	})
}

func testNewAPIAccount(baseURL string) accountQuery {
	return accountQuery{
		ID:          "account-1",
		ProviderKey: "provider-1",
		AccountName: "New API 测试",
		BaseURL:     baseURL,
		APIKey:      "sk-test-secret",
		QueryType:   balance.ProviderNewAPI,
	}
}

type fetcherFunc func(authID, token, proxyURL string) balance.Result

func (fn fetcherFunc) Fetch(authID, token, proxyURL string) balance.Result {
	return fn(authID, token, proxyURL)
}

func useFetcherForTest(t *testing.T, fetcher balance.Fetcher) {
	t.Helper()
	previous := buildFetcher
	buildFetcher = func(balance.ProviderType, string) balance.Fetcher { return fetcher }
	t.Cleanup(func() { buildFetcher = previous })
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
