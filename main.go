package main

/*
#include <stdint.h>
#include <stdlib.h>

typedef struct {
	void* ptr;
	size_t len;
} cliproxy_buffer;

typedef int (*cliproxy_host_call_fn)(void*, const char*, const uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_host_free_fn)(void*, size_t);

typedef struct {
	uint32_t abi_version;
	void* host_ctx;
	cliproxy_host_call_fn call;
	cliproxy_host_free_fn free_buffer;
} cliproxy_host_api;

typedef int (*cliproxy_plugin_call_fn)(char*, uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_plugin_free_fn)(void*, size_t);
typedef void (*cliproxy_plugin_shutdown_fn)(void);

typedef struct {
	uint32_t abi_version;
	cliproxy_plugin_call_fn call;
	cliproxy_plugin_free_fn free_buffer;
	cliproxy_plugin_shutdown_fn shutdown;
} cliproxy_plugin_api;

extern int cliproxyPluginCall(char*, uint8_t*, size_t, cliproxy_buffer*);
extern void cliproxyPluginFree(void*, size_t);
extern void cliproxyPluginShutdown(void);
*/
import "C"

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/Hamster-Prime/balance-query/internal/balance"
	"github.com/Hamster-Prime/balance-query/internal/cache"
	"github.com/Hamster-Prime/balance-query/internal/providers"
	"github.com/Hamster-Prime/balance-query/internal/ui"
	"gopkg.in/yaml.v3"
)

const (
	pluginID      = "balance-query"
	pluginVersion = "0.8.2"
	abiVersion    = 1
	schemaVersion = 1

	resourcePath = "/dashboard"
	queryPath    = "/" + pluginID + "/query"

	defaultTTLSeconds     = 300
	maxQueryAccounts      = 128
	maxQueryBodyBytes     = 1 << 20
	maxPluginRequestBytes = 8 << 20
)

var (
	resultCache  = cache.New[string, balance.Result](defaultTTLSeconds * time.Second)
	querySlots   = make(chan struct{}, 8)
	fetchMu      sync.Mutex
	fetchCalls   = map[string]*accountFetchCall{}
	buildFetcher = providers.Build

	stateMu sync.RWMutex
	state   = balance.PluginConfig{
		CacheTTLSeconds:  defaultTTLSeconds,
		ProviderMappings: map[string]balance.ProviderType{},
	}
)

type accountFetchCall struct {
	done   chan struct{}
	result balance.Result
}

type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *envelopeError  `json:"error,omitempty"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func main() {}

//export cliproxy_plugin_init
func cliproxy_plugin_init(_ *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
	if plugin == nil {
		return 1
	}
	plugin.abi_version = C.uint32_t(abiVersion)
	plugin.call = C.cliproxy_plugin_call_fn(C.cliproxyPluginCall)
	plugin.free_buffer = C.cliproxy_plugin_free_fn(C.cliproxyPluginFree)
	plugin.shutdown = C.cliproxy_plugin_shutdown_fn(C.cliproxyPluginShutdown)
	return 0
}

//export cliproxyPluginCall
func cliproxyPluginCall(method *C.char, request *C.uint8_t, requestLen C.size_t, response *C.cliproxy_buffer) (returnCode C.int) {
	if response == nil {
		return 1
	}
	response.ptr = nil
	response.len = 0
	defer func() {
		if recovered := recover(); recovered != nil {
			writeResponse(response, errorEnvelope("plugin_panic", "插件处理请求时发生内部错误"))
			returnCode = 1
		}
	}()
	if method == nil {
		writeResponse(response, errorEnvelope("invalid_method", "缺少插件调用方法"))
		return 1
	}
	if requestLen > C.size_t(maxPluginRequestBytes) {
		writeResponse(response, errorEnvelope("request_too_large", "插件请求超过 8 MiB 安全上限"))
		return 1
	}
	if request == nil && requestLen > 0 {
		writeResponse(response, errorEnvelope("invalid_request", "插件请求指针与长度不一致"))
		return 1
	}

	var requestBytes []byte
	if request != nil && requestLen > 0 {
		requestBytes = C.GoBytes(unsafe.Pointer(request), C.int(requestLen))
	}
	raw, err := handleMethod(C.GoString(method), requestBytes)
	if err != nil {
		writeResponse(response, errorEnvelope("plugin_error", err.Error()))
		return 1
	}
	writeResponse(response, raw)
	return 0
}

//export cliproxyPluginFree
func cliproxyPluginFree(ptr unsafe.Pointer, _ C.size_t) {
	if ptr != nil {
		C.free(ptr)
	}
}

//export cliproxyPluginShutdown
func cliproxyPluginShutdown() {
	providers.CloseIdleConnections()
	stateMu.Lock()
	state = balance.PluginConfig{
		CacheTTLSeconds:  defaultTTLSeconds,
		ProviderMappings: map[string]balance.ProviderType{},
	}
	stateMu.Unlock()
	resultCache.Reset(defaultTTLSeconds * time.Second)
}

func handleMethod(method string, request []byte) ([]byte, error) {
	switch method {
	case "plugin.register", "plugin.reconfigure":
		return handleRegister(request)
	case "management.register":
		return handleManagementRegister()
	case "management.handle":
		return handleManagementRequest(request)
	default:
		return errorEnvelope("unknown_method", "不支持的插件方法："+method), nil
	}
}

type lifecycleRequest struct {
	ConfigYAML    []byte `json:"config_yaml"`
	SchemaVersion uint32 `json:"schema_version"`
}

type registration struct {
	SchemaVersion uint32                   `json:"schema_version"`
	Metadata      pluginMetadata           `json:"metadata"`
	Capabilities  registrationCapabilities `json:"capabilities"`
}

type pluginMetadata struct {
	Name             string        `json:"Name"`
	Version          string        `json:"Version"`
	Author           string        `json:"Author"`
	GitHubRepository string        `json:"GitHubRepository"`
	Logo             string        `json:"Logo"`
	ConfigFields     []configField `json:"ConfigFields"`
}

type configField struct {
	Name        string   `json:"Name"`
	Type        string   `json:"Type"`
	EnumValues  []string `json:"EnumValues,omitempty"`
	Description string   `json:"Description"`
}

type registrationCapabilities struct {
	ManagementAPI bool `json:"management_api"`
}

func handleRegister(raw []byte) ([]byte, error) {
	if err := applyRuntimeConfig(raw); err != nil {
		return nil, err
	}
	return okEnvelope(registration{
		SchemaVersion: schemaVersion,
		Metadata: pluginMetadata{
			Name:             "余额与配额",
			Version:          pluginVersion,
			Author:           "Hamster-Prime",
			GitHubRepository: "https://github.com/Hamster-Prime/balance-query",
			Logo:             "https://raw.githubusercontent.com/Hamster-Prime/balance-query/main/assets/logo.png",
			ConfigFields: []configField{
				{
					Name:        "cache_ttl_seconds",
					Type:        "integer",
					Description: "余额查询缓存时长（秒）；设为 0 可关闭缓存。",
				},
				{
					Name:        "provider_mappings",
					Type:        "object",
					Description: "AI 提供商与余额查询类型的映射，由插件页面维护。",
				},
			},
		},
		Capabilities: registrationCapabilities{ManagementAPI: true},
	})
}

func applyRuntimeConfig(raw []byte) error {
	if len(raw) == 0 {
		return nil
	}
	var request lifecycleRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return fmt.Errorf("解析插件配置请求失败：%w", err)
	}
	if len(request.ConfigYAML) == 0 || strings.TrimSpace(string(request.ConfigYAML)) == "" {
		stateMu.Lock()
		state = balance.PluginConfig{
			CacheTTLSeconds:  defaultTTLSeconds,
			ProviderMappings: map[string]balance.ProviderType{},
		}
		stateMu.Unlock()
		resultCache.Reset(defaultTTLSeconds * time.Second)
		return nil
	}

	var decoded struct {
		CacheTTLSeconds  *int                            `yaml:"cache_ttl_seconds"`
		ProviderMappings map[string]balance.ProviderType `yaml:"provider_mappings"`
	}
	if err := yaml.Unmarshal(request.ConfigYAML, &decoded); err != nil {
		return fmt.Errorf("解析插件配置失败：%w", err)
	}
	next := balance.PluginConfig{
		CacheTTLSeconds:  defaultTTLSeconds,
		ProviderMappings: decoded.ProviderMappings,
	}
	if decoded.CacheTTLSeconds != nil {
		next.CacheTTLSeconds = normalizeTTL(*decoded.CacheTTLSeconds)
	}
	if next.ProviderMappings == nil {
		next.ProviderMappings = map[string]balance.ProviderType{}
	}
	for key, providerType := range next.ProviderMappings {
		if !balance.IsKnownProvider(providerType) {
			delete(next.ProviderMappings, key)
		}
	}

	stateMu.Lock()
	previousTTL := state.CacheTTLSeconds
	state = next
	stateMu.Unlock()

	if previousTTL != next.CacheTTLSeconds {
		resultCache.Reset(time.Duration(next.CacheTTLSeconds) * time.Second)
	}
	return nil
}

func normalizeTTL(value int) int {
	if value == 0 {
		return 0
	}
	if value < 10 {
		return 10
	}
	if value > 86400 {
		return 86400
	}
	return value
}

func currentTTL() int {
	stateMu.RLock()
	defer stateMu.RUnlock()
	return state.CacheTTLSeconds
}

type managementRegistration struct {
	Routes    []managementRoute    `json:"routes,omitempty"`
	Resources []managementResource `json:"resources,omitempty"`
}

type managementRoute struct {
	Method string `json:"Method"`
	Path   string `json:"Path"`
}

type managementResource struct {
	Path        string `json:"Path"`
	Menu        string `json:"Menu"`
	Description string `json:"Description"`
}

type managementRequest struct {
	Method  string      `json:"Method"`
	Path    string      `json:"Path"`
	Headers http.Header `json:"Headers"`
	Query   url.Values  `json:"Query"`
	Body    []byte      `json:"Body"`
}

type managementResponse struct {
	StatusCode int         `json:"StatusCode"`
	Headers    http.Header `json:"Headers"`
	Body       []byte      `json:"Body"`
}

func handleManagementRegister() ([]byte, error) {
	return okEnvelope(managementRegistration{
		Routes: []managementRoute{{Method: http.MethodPost, Path: queryPath}},
		Resources: []managementResource{{
			Path:        resourcePath,
			Menu:        "余额与配额",
			Description: "查看 AI 提供商的余额、用量与套餐配额",
		}},
	})
}

func handleManagementRequest(raw []byte) ([]byte, error) {
	var request managementRequest
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &request); err != nil {
			return nil, fmt.Errorf("解析管理请求失败：%w", err)
		}
	}

	if strings.HasSuffix(request.Path, queryPath) {
		if !strings.EqualFold(request.Method, http.MethodPost) {
			return okEnvelope(jsonResponse(http.StatusMethodNotAllowed, map[string]string{
				"error": "仅支持 POST 请求",
			}))
		}
		return handleBalanceQuery(request)
	}

	page := ui.RenderDashboard(currentTTL())
	return okEnvelope(htmlResponse(http.StatusOK, page))
}

type accountQuery struct {
	ID          string               `json:"id"`
	ProviderKey string               `json:"provider_key"`
	MappingKey  string               `json:"mapping_key,omitempty"`
	AccountName string               `json:"account_name"`
	BaseURL     string               `json:"base_url"`
	APIKey      string               `json:"api_key"`
	ProxyURL    string               `json:"proxy_url,omitempty"`
	QueryType   balance.ProviderType `json:"query_type"`
}

type balanceQueryRequest struct {
	Accounts []accountQuery `json:"accounts"`
	Refresh  bool           `json:"refresh"`
}

type balanceQueryResponse struct {
	Results    []balance.Result `json:"results"`
	FetchedAt  time.Time        `json:"fetched_at"`
	TTLSeconds int              `json:"ttl_seconds"`
}

func handleBalanceQuery(request managementRequest) ([]byte, error) {
	if len(request.Body) > maxQueryBodyBytes {
		return okEnvelope(jsonResponse(http.StatusRequestEntityTooLarge, map[string]string{
			"error": "查询请求过大",
		}))
	}
	var query balanceQueryRequest
	if err := json.Unmarshal(request.Body, &query); err != nil {
		return okEnvelope(jsonResponse(http.StatusBadRequest, map[string]string{
			"error": "查询参数格式不正确",
		}))
	}
	if len(query.Accounts) > maxQueryAccounts {
		return okEnvelope(jsonResponse(http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("单次最多查询 %d 个密钥", maxQueryAccounts),
		}))
	}

	for i := range query.Accounts {
		normalizeAccountQuery(&query.Accounts[i])
		if err := validateAccountQueryShape(query.Accounts[i]); err != nil {
			return okEnvelope(jsonResponse(http.StatusBadRequest, map[string]string{
				"error": fmt.Sprintf("第 %d 个查询项无效：%s", i+1, err),
			}))
		}
		if err := resolveConfiguredQueryType(&query.Accounts[i]); err != nil {
			return okEnvelope(jsonResponse(http.StatusConflict, map[string]string{
				"error": fmt.Sprintf("第 %d 个查询项配置已变化：%s", i+1, err),
			}))
		}
		if err := validateResolvedAccountQuery(query.Accounts[i]); err != nil {
			return okEnvelope(jsonResponse(http.StatusBadRequest, map[string]string{
				"error": fmt.Sprintf("第 %d 个查询项无效：%s", i+1, err),
			}))
		}
	}

	results := fetchAccounts(query.Accounts, query.Refresh)
	return okEnvelope(jsonResponse(http.StatusOK, balanceQueryResponse{
		Results:    results,
		FetchedAt:  time.Now(),
		TTLSeconds: currentTTL(),
	}))
}

func normalizeAccountQuery(account *accountQuery) {
	if account == nil {
		return
	}
	account.ID = strings.TrimSpace(account.ID)
	account.ProviderKey = strings.TrimSpace(account.ProviderKey)
	account.MappingKey = strings.TrimSpace(account.MappingKey)
	account.AccountName = strings.TrimSpace(account.AccountName)
	account.BaseURL = strings.TrimSpace(account.BaseURL)
	account.APIKey = strings.TrimSpace(account.APIKey)
	account.ProxyURL = strings.TrimSpace(account.ProxyURL)
	account.QueryType = balance.ProviderType(strings.TrimSpace(string(account.QueryType)))
}

func validateAccountQuery(account accountQuery) error {
	if err := validateAccountQueryShape(account); err != nil {
		return err
	}
	return validateResolvedAccountQuery(account)
}

func validateAccountQueryShape(account accountQuery) error {
	if strings.TrimSpace(account.ID) == "" || len(account.ID) > 256 {
		return fmt.Errorf("缺少有效的账户标识")
	}
	if strings.TrimSpace(account.ProviderKey) == "" || len(account.ProviderKey) > 4096 {
		return fmt.Errorf("缺少有效的提供商标识")
	}
	if strings.TrimSpace(account.AccountName) == "" || len(account.AccountName) > 256 {
		return fmt.Errorf("缺少有效的提供商名称")
	}
	if strings.TrimSpace(account.APIKey) == "" || len(account.APIKey) > 8192 {
		return fmt.Errorf("接口密钥为空或过长")
	}
	if len(account.MappingKey) > 4096 {
		return fmt.Errorf("提供商映射标识过长")
	}
	if len(account.BaseURL) > 4096 {
		return fmt.Errorf("接口地址过长")
	}
	if strings.TrimSpace(account.BaseURL) != "" {
		parsed, err := url.Parse(strings.TrimSpace(account.BaseURL))
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
			return fmt.Errorf("接口地址必须是有效的 HTTP(S) URL")
		}
	}
	if err := validateProxyURL(account.ProxyURL); err != nil {
		return err
	}
	return nil
}

func validateResolvedAccountQuery(account accountQuery) error {
	if !balance.IsKnownProvider(account.QueryType) {
		return fmt.Errorf("未知的余额查询类型")
	}
	if providerRequiresBaseURL(account.QueryType) && strings.TrimSpace(account.BaseURL) == "" {
		return fmt.Errorf("该查询类型需要有效的 HTTP(S) URL 接口地址")
	}
	return nil
}

func providerRequiresBaseURL(providerType balance.ProviderType) bool {
	return providerType == balance.ProviderSub2API || providerType == balance.ProviderNewAPI
}

func resolveConfiguredQueryType(account *accountQuery) error {
	if account == nil {
		return fmt.Errorf("查询项为空")
	}
	mappingKey := strings.TrimSpace(account.MappingKey)
	if mappingKey == "" {
		mappingKey = account.ProviderKey
	}
	stateMu.RLock()
	configuredType, ok := state.ProviderMappings[mappingKey]
	stateMu.RUnlock()
	if !ok || !balance.IsKnownProvider(configuredType) {
		return fmt.Errorf("当前配置中没有此提供商映射，请重新载入页面")
	}
	if account.QueryType != "" && account.QueryType != configuredType {
		return fmt.Errorf("查询类型已被其他页面修改，请重新载入页面")
	}
	account.MappingKey = mappingKey
	account.QueryType = configuredType
	return nil
}

func validateProxyURL(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	if len(trimmed) > 4096 {
		return fmt.Errorf("代理地址过长")
	}
	if strings.EqualFold(trimmed, "direct") || strings.EqualFold(trimmed, "none") {
		return nil
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("代理地址无效")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "socks5", "socks5h":
		return nil
	default:
		return fmt.Errorf("代理地址协议不受支持")
	}
}

func fetchAccounts(accounts []accountQuery, refresh bool) []balance.Result {
	results := make([]balance.Result, len(accounts))
	var wait sync.WaitGroup
	for index := range accounts {
		account := accounts[index]
		cacheKey := accountCacheKey(account)
		if !refresh {
			if cached, ok := resultCache.Get(cacheKey); ok {
				results[index] = decorateAccountResult(cached, account)
				continue
			}
		}

		wait.Add(1)
		go func(resultIndex int, item accountQuery, key string) {
			defer wait.Done()
			results[resultIndex] = fetchAccountCoalesced(item, key)
		}(index, account, cacheKey)
	}
	wait.Wait()
	return results
}

func fetchAccountCoalesced(account accountQuery, cacheKey string) balance.Result {
	fetchMu.Lock()
	if call, ok := fetchCalls[cacheKey]; ok {
		fetchMu.Unlock()
		<-call.done
		return decorateAccountResult(call.result, account)
	}
	call := &accountFetchCall{done: make(chan struct{})}
	fetchCalls[cacheKey] = call
	fetchMu.Unlock()

	result := performAccountFetch(account)
	cacheFetchResult(cacheKey, result)

	fetchMu.Lock()
	call.result = result
	delete(fetchCalls, cacheKey)
	close(call.done)
	fetchMu.Unlock()
	return decorateAccountResult(result, account)
}

func performAccountFetch(account accountQuery) (result balance.Result) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = accountError(account, "余额查询器发生内部错误")
		}
	}()
	querySlots <- struct{}{}
	defer func() { <-querySlots }()
	return fetchAccount(account)
}

func cacheFetchResult(cacheKey string, result balance.Result) {
	ttlSeconds := currentTTL()
	if ttlSeconds <= 0 {
		return
	}
	if result.Error == "" && len(result.Warnings) == 0 {
		resultCache.Set(cacheKey, result)
		return
	}
	// Preserve a still-valid successful snapshot when a manual refresh has a
	// transient failure. Otherwise cache failures briefly to avoid repeatedly
	// hammering an invalid key or unavailable endpoint on every page entry.
	if existing, ok := resultCache.Get(cacheKey); ok && existing.Error == "" && len(existing.Warnings) == 0 {
		return
	}
	failureTTL := 30 * time.Second
	if result.Error == "" && len(result.Warnings) > 0 {
		failureTTL = 20 * time.Second
	}
	if result.Failure != nil {
		switch result.Failure.Kind {
		case balance.FailureAuthentication, balance.FailurePermission,
			balance.FailureInsufficientFund, balance.FailureQuotaExhausted,
			balance.FailureAccount,
			balance.FailureInvalidConfig, balance.FailureEndpoint,
			balance.FailureUnsupported, balance.FailureNoData:
			failureTTL = 2 * time.Minute
		case balance.FailureRateLimited, balance.FailureTimeout,
			balance.FailureConflict,
			balance.FailureNetwork, balance.FailureDNS, balance.FailureTLS,
			balance.FailureProxy, balance.FailureService:
			failureTTL = 20 * time.Second
		}
	}
	configuredTTL := time.Duration(ttlSeconds) * time.Second
	if failureTTL > configuredTTL {
		failureTTL = configuredTTL
	}
	resultCache.SetWithTTL(cacheKey, result, failureTTL)
}

func decorateAccountResult(result balance.Result, account accountQuery) balance.Result {
	result = cloneBalanceResult(result)
	result.ProviderKey = account.ProviderKey
	result.AuthID = account.ID
	result.AccountName = account.AccountName
	result.KeyPreview = maskAPIKey(account.APIKey)
	result.BaseURL = account.BaseURL
	redactResultSecret(&result, account.APIKey)
	return result
}

func cloneBalanceResult(result balance.Result) balance.Result {
	cloned := result
	if result.Failure != nil {
		failure := *result.Failure
		cloned.Failure = &failure
	}
	cloned.Warnings = append([]balance.FailureInfo(nil), result.Warnings...)
	cloned.QuotaWindows = append([]balance.QuotaWindow(nil), result.QuotaWindows...)
	if result.Extra != nil {
		cloned.Extra = make(map[string]string, len(result.Extra))
		for key, value := range result.Extra {
			cloned.Extra[key] = value
		}
	}
	return cloned
}

func fetchAccount(account accountQuery) balance.Result {
	fetcher := buildFetcher(account.QueryType, account.BaseURL)
	if fetcher == nil {
		return accountError(account, "未找到对应的余额查询器")
	}
	result := fetcher.Fetch(account.ID, account.APIKey, account.ProxyURL)
	result = decorateAccountResult(result, account)
	normalizeResultFailure(&result, account.QueryType)
	return result
}

func accountError(account accountQuery, message string) balance.Result {
	result := balance.Result{
		Provider:    balance.ProviderLabel[account.QueryType],
		ProviderKey: account.ProviderKey,
		AuthID:      account.ID,
		AccountName: account.AccountName,
		KeyPreview:  maskAPIKey(account.APIKey),
		BaseURL:     account.BaseURL,
		Error:       message,
		FetchedAt:   time.Now(),
	}
	redactResultSecret(&result, account.APIKey)
	normalizeResultFailure(&result, account.QueryType)
	return result
}

func maskAPIKey(value string) string {
	key := strings.TrimSpace(value)
	if len(key) <= 6 {
		if len(key) <= 2 {
			return "••••"
		}
		return "••••" + key[len(key)-2:]
	}
	return key[:3] + "••••••" + key[len(key)-4:]
}

func redactResultSecret(result *balance.Result, secret string) {
	if result == nil {
		return
	}
	redact := func(value string) string {
		masked := maskAPIKey(secret)
		for _, candidate := range []string{secret, strings.TrimSpace(secret)} {
			if candidate != "" {
				value = strings.ReplaceAll(value, candidate, masked)
			}
		}
		return value
	}
	result.Provider = redact(result.Provider)
	result.ProviderKey = redact(result.ProviderKey)
	result.AuthID = redact(result.AuthID)
	result.AccountName = redact(result.AccountName)
	result.KeyPreview = redact(result.KeyPreview)
	result.BaseURL = redact(result.BaseURL)
	result.BalanceScope = redact(result.BalanceScope)
	result.BalanceCurrency = redact(result.BalanceCurrency)
	result.CostScope = redact(result.CostScope)
	result.Error = redact(result.Error)
	result.QuotaDisplay = redact(result.QuotaDisplay)
	result.Plan = redact(result.Plan)
	result.ResetAt = redact(result.ResetAt)
	redactFailure := func(failure *balance.FailureInfo) {
		if failure == nil {
			return
		}
		failure.Kind = redact(failure.Kind)
		failure.Title = redact(failure.Title)
		failure.Reason = redact(failure.Reason)
		failure.Suggestion = redact(failure.Suggestion)
		failure.ProviderCode = redact(failure.ProviderCode)
		failure.RequestID = redact(failure.RequestID)
	}
	redactFailure(result.Failure)
	for index := range result.Warnings {
		redactFailure(&result.Warnings[index])
	}
	if len(result.Extra) > 0 {
		redactedExtra := make(map[string]string, len(result.Extra))
		for key, value := range result.Extra {
			redactedExtra[redact(key)] = redact(value)
		}
		result.Extra = redactedExtra
	}
	for index := range result.QuotaWindows {
		window := &result.QuotaWindows[index]
		window.Group = redact(window.Group)
		window.Label = redact(window.Label)
		window.Unit = redact(window.Unit)
		window.ResetAt = redact(window.ResetAt)
		window.Status = redact(window.Status)
		window.AggregationScope = redact(window.AggregationScope)
		window.AggregationKey = redact(window.AggregationKey)
	}
}

func localizeFetchError(message string) string {
	failure := classifyFetchFailure("", message, nil)
	if failure == nil {
		return ""
	}
	return failure.Reason
}

func normalizeResultFailure(result *balance.Result, providerType balance.ProviderType) {
	if result == nil {
		return
	}
	normalizedWarnings := result.Warnings[:0]
	for index := range result.Warnings {
		warning := result.Warnings[index]
		message := strings.TrimSpace(warning.Reason)
		if message == "" {
			message = warning.Title
		}
		normalized := classifyFetchFailure(providerType, message, &warning)
		if normalized == nil {
			continue
		}
		if contextTitle := partialFailureContext(warning.Title); contextTitle != "" {
			normalized.Title = contextTitle + " · " + normalized.Title
		}
		normalizedWarnings = append(normalizedWarnings, *normalized)
	}
	result.Warnings = normalizedWarnings
	if strings.TrimSpace(result.Error) == "" {
		result.Failure = nil
		return
	}
	result.Failure = classifyFetchFailure(providerType, result.Error, result.Failure)
	result.Error = result.Failure.Reason
}

func partialFailureContext(title string) string {
	trimmed := strings.TrimSpace(title)
	for _, separator := range []string{" · ", "：", ":"} {
		if index := strings.Index(trimmed, separator); index > 0 {
			prefix := strings.TrimSpace(trimmed[:index])
			if strings.Contains(prefix, "查询") || strings.Contains(prefix, "统计") {
				return prefix
			}
		}
	}
	if strings.Contains(trimmed, "查询") || strings.Contains(trimmed, "统计") {
		return trimmed
	}
	return ""
}

func classifyFetchFailure(providerType balance.ProviderType, message string, existing *balance.FailureInfo) *balance.FailureInfo {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return nil
	}
	failure := balance.FailureInfo{}
	if existing != nil {
		failure = *existing
	}
	if failure.HTTPStatus == 0 {
		failure.HTTPStatus = httpStatusFromMessage(trimmed)
	}
	if strings.TrimSpace(failure.Kind) == "" || failure.Kind == balance.FailureUnknown {
		failure.Kind = inferFailureKind(providerType, failure.HTTPStatus, failure.ProviderCode, trimmed)
	}
	failure.Title = failureTitle(failure.Kind)
	failure.Reason = failureReason(providerType, failure.Kind, failure.ProviderCode, trimmed)
	if failure.RetryAfterSeconds > 0 {
		failure.Suggestion = fmt.Sprintf("请在 %d 秒后重试。", failure.RetryAfterSeconds)
	} else if suggestion := failureSuggestion(providerType, failure.Kind, failure.ProviderCode); suggestion != "" {
		failure.Suggestion = suggestion
	}
	failure.Retryable = failure.RetryAfterSeconds > 0 || failureRetryable(failure.Kind)
	return &failure
}

func inferFailureKind(providerType balance.ProviderType, status int, providerCode, message string) string {
	code := strings.ToLower(strings.TrimSpace(providerCode))
	lower := strings.ToLower(message)
	if kind := providerFailureKind(providerType, code); kind != "" {
		return kind
	}
	switch code {
	case "api_key_required", "invalid_api_key", "unauthorized", "authentication_error", "invalid_api_key_error":
		return balance.FailureAuthentication
	case "api_key_disabled", "user_not_found":
		return balance.FailureAccount
	case "access_denied", "permission_error", "forbidden":
		return balance.FailurePermission
	case "engine_overloaded_error", "overloaded_error":
		return balance.FailureService
	case "exceeded_current_quota_error", "insufficient_quota", "insufficient_balance":
		return balance.FailureQuotaExhausted
	case "rate_limit_reached_error", "rate_limit_error", "api_key_auth_overloaded":
		return balance.FailureRateLimited
	}
	switch status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity, http.StatusRequestEntityTooLarge:
		return balance.FailureInvalidConfig
	case http.StatusUnauthorized:
		return balance.FailureAuthentication
	case http.StatusPaymentRequired:
		return balance.FailureInsufficientFund
	case http.StatusForbidden:
		return balance.FailurePermission
	case http.StatusNotFound, http.StatusMethodNotAllowed:
		return balance.FailureEndpoint
	case http.StatusConflict:
		return balance.FailureConflict
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return balance.FailureTimeout
	case http.StatusTooManyRequests:
		return balance.FailureRateLimited
	}
	if status >= 500 {
		return balance.FailureService
	}
	switch {
	case isAuditedUnsupportedReason(message):
		return balance.FailureUnsupported
	case containsAny(lower, "所配置代理", "代理地址", "proxyconnect", "proxy error"):
		return balance.FailureProxy
	case containsAny(lower, "context deadline", "deadline exceeded", "timeout", "timed out"):
		return balance.FailureTimeout
	case containsAny(lower, "no such host", "server misbehaving", "temporary failure in name resolution", "lookup "):
		return balance.FailureDNS
	case containsAny(lower, "tls", "x509", "certificate"):
		return balance.FailureTLS
	case containsAny(lower, "connection refused", "connection reset", "broken pipe", "dial tcp", "network is unreachable", "请求余额接口失败"):
		return balance.FailureNetwork
	case containsAny(lower, "解析余额接口响应失败", "invalid character", "unexpected end of json input", "无法识别的数据", "响应超过 2 mib"):
		return balance.FailureInvalidResponse
	case containsAny(lower, "接口密钥为空", "没有配置接口地址", "接口地址无效", "创建请求失败", "未知的余额查询类型"):
		return balance.FailureInvalidConfig
	case containsAny(lower, "未返回", "没有可识别", "无可识别"):
		return balance.FailureNoData
	case containsAny(lower, "unauthorized", "invalid api key", "invalid_api_key", "credential", "token invalid"):
		return balance.FailureAuthentication
	case containsAny(lower, "forbidden", "permission", "access denied"):
		return balance.FailurePermission
	case containsAny(lower, "insufficient balance", "余额不足", "payment required"):
		return balance.FailureInsufficientFund
	case containsAny(lower, "quota exhausted", "usage limit", "额度耗尽", "套餐已过期"):
		return balance.FailureQuotaExhausted
	case containsAny(lower, "too many requests", "rate limit", "请求过于频繁"):
		return balance.FailureRateLimited
	default:
		return balance.FailureUnknown
	}
}

func providerFailureKind(providerType balance.ProviderType, code string) string {
	switch providerType {
	case balance.ProviderMiniMaxAPI, balance.ProviderMiniMaxCodingCN, balance.ProviderMiniMaxCodingGlobal:
		switch code {
		case "1001":
			return balance.FailureTimeout
		case "1002", "1041":
			return balance.FailureRateLimited
		case "1004", "2049":
			return balance.FailureAuthentication
		case "1008":
			return balance.FailureInsufficientFund
		case "1039", "2056":
			return balance.FailureQuotaExhausted
		case "1024", "1033":
			return balance.FailureService
		}
	case balance.ProviderGLMZAI, balance.ProviderGLMZhipu:
		switch code {
		case "1000", "1001", "1003", "1315":
			return balance.FailureAuthentication
		case "1113":
			return balance.FailureInsufficientFund
		case "1220", "1311":
			return balance.FailurePermission
		case "1302", "1313":
			return balance.FailureRateLimited
		case "1305":
			return balance.FailureService
		case "1308", "1309", "1310", "1316", "1317", "1318", "1319", "1320", "1321":
			return balance.FailureQuotaExhausted
		}
	}
	return ""
}

func failureTitle(kind string) string {
	switch kind {
	case balance.FailureAuthentication:
		return "认证失败"
	case balance.FailurePermission:
		return "权限不足"
	case balance.FailureInsufficientFund:
		return "余额不足"
	case balance.FailureQuotaExhausted:
		return "额度已耗尽"
	case balance.FailureRateLimited:
		return "请求受限"
	case balance.FailureConflict:
		return "请求冲突"
	case balance.FailureInvalidConfig:
		return "配置错误"
	case balance.FailureEndpoint:
		return "接口不匹配"
	case balance.FailureProxy:
		return "代理连接失败"
	case balance.FailureTimeout:
		return "请求超时"
	case balance.FailureDNS:
		return "域名解析失败"
	case balance.FailureTLS:
		return "TLS 校验失败"
	case balance.FailureNetwork:
		return "网络连接失败"
	case balance.FailureInvalidResponse:
		return "响应格式异常"
	case balance.FailureService:
		return "服务暂不可用"
	case balance.FailureAccount:
		return "账户不可用"
	case balance.FailureNoData:
		return "暂无可查询数据"
	case balance.FailureUnsupported:
		return "暂不支持自动查询"
	default:
		return "查询失败"
	}
}

func failureReason(providerType balance.ProviderType, kind, providerCode, original string) string {
	code := strings.ToLower(strings.TrimSpace(providerCode))
	if isAuditedUnsupportedReason(original) || kind == balance.FailureNoData {
		return original
	}
	if kind == balance.FailureInvalidResponse && containsAny(original, "未返回", "无法识别", "响应超过 2 MiB") {
		return original
	}
	if providerType == balance.ProviderMiniMaxCodingCN || providerType == balance.ProviderMiniMaxCodingGlobal {
		switch code {
		case "2056":
			return "当前 5 小时 Token Plan 使用窗口已达上限"
		case "1008":
			return "MiniMax 账户余额不足"
		}
	}
	if providerType == balance.ProviderGLMZAI || providerType == balance.ProviderGLMZhipu {
		switch code {
		case "1309":
			return "GLM Coding Plan 套餐已过期"
		case "1315":
			return "当前密钥类型不适用于 GLM Coding Plan 查询"
		}
	}
	if code == "engine_overloaded_error" {
		return "模型服务当前过载"
	}
	if code == "exceeded_current_quota_error" {
		return "账户余额不足、服务已停用，或当前额度已经用尽"
	}
	switch kind {
	case balance.FailureAuthentication:
		return "余额接口拒绝了密钥：密钥无效、已过期，或与当前查询类型及区域不匹配"
	case balance.FailurePermission:
		return "账户或密钥没有访问余额与用量接口的权限"
	case balance.FailureInsufficientFund:
		return "账户余额不足，余额接口拒绝继续提供服务"
	case balance.FailureQuotaExhausted:
		return "当前套餐额度已用尽或套餐已过期"
	case balance.FailureRateLimited:
		return "查询触发了频率、并发或公平使用限制"
	case balance.FailureConflict:
		return "上游服务检测到并发操作冲突"
	case balance.FailureInvalidConfig:
		return "查询参数、密钥类型或接口配置不正确"
	case balance.FailureEndpoint:
		return "当前服务未提供该余额接口，或接口地址与查询类型不匹配"
	case balance.FailureProxy:
		return "无法通过所配置的代理连接余额服务"
	case balance.FailureTimeout:
		return "余额服务在超时时间内没有完成响应"
	case balance.FailureDNS:
		return "无法解析余额服务的域名"
	case balance.FailureTLS:
		return "余额服务的 TLS 证书或安全连接校验失败"
	case balance.FailureNetwork:
		return "无法连接余额服务"
	case balance.FailureInvalidResponse:
		return "余额接口返回了空数据、非 JSON 或无法识别的字段"
	case balance.FailureService:
		return "余额服务暂时不可用或正在过载"
	case balance.FailureAccount:
		return "查询成功，但账户当前不可用或已被限制"
	case balance.FailureUnsupported:
		return original
	default:
		return "余额查询未成功，请检查查询类型、接口地址和账户状态"
	}
}

func failureSuggestion(providerType balance.ProviderType, kind, providerCode string) string {
	switch providerType {
	case balance.ProviderClaudeAdmin:
		if kind == balance.FailureAuthentication || kind == balance.FailurePermission {
			return "请使用 Anthropic Admin API 密钥（不是普通模型 API Key），并确认组织用量与费用读取权限。"
		}
	case balance.ProviderKimiAPI:
		if kind == balance.FailureAuthentication || kind == balance.FailureEndpoint {
			return "请确认国内站与国际站的 Key、账户和接口地址一致；两站数据不互通。"
		}
	case balance.ProviderKimiCode:
		if kind == balance.FailureAuthentication || kind == balance.FailurePermission {
			return "请使用 Kimi Coding Plan 的访问凭证，并确认套餐仍有效。"
		}
	case balance.ProviderMiniMaxCodingCN, balance.ProviderMiniMaxCodingGlobal:
		if kind == balance.FailureAuthentication || kind == balance.FailurePermission || kind == balance.FailureInvalidConfig {
			return "请使用对应区域的 Token Plan Subscription Key（不是按量 API Key），并确认已分配有效席位或 Credits。"
		}
	case balance.ProviderGLMZAI, balance.ProviderGLMZhipu:
		if kind == balance.FailureAuthentication || kind == balance.FailurePermission || strings.TrimSpace(providerCode) == "1315" {
			return "请核对 Z.AI 与智谱 BigModel 区域，并使用对应 Coding Plan 的正确密钥类型。"
		}
	case balance.ProviderNewAPI:
		if kind == balance.FailureEndpoint || kind == balance.FailureInvalidConfig {
			return "请确认这是兼容的 New API 实例，并填写实例的 OpenAI 兼容 Base URL。"
		}
	case balance.ProviderSub2API:
		if kind == balance.FailureAuthentication || kind == balance.FailurePermission {
			return "请检查 Sub2API Key 是否启用、用户状态、IP 白名单及所属分组权限。"
		}
	case balance.ProviderDeepSeek:
		if kind == balance.FailureAuthentication {
			return "请使用 DeepSeek 官方 API Key，并核对官方接口地址。"
		}
	}
	switch kind {
	case balance.FailureRateLimited, balance.FailureConflict, balance.FailureService, balance.FailureTimeout:
		return "请稍后重试；手动刷新不会清除上一次成功缓存。"
	case balance.FailureProxy:
		return "请检查此密钥的代理协议、地址和网络连通性，或改为直连。"
	case balance.FailureDNS, balance.FailureTLS, balance.FailureNetwork:
		return "请检查网络、DNS、证书、代理及接口地址。"
	case balance.FailureInsufficientFund:
		return "请充值或切换到仍有余额的账户。"
	case balance.FailureQuotaExhausted:
		return "请等待额度窗口重置、续费套餐或启用额外用量。"
	case balance.FailureEndpoint:
		return "请重新选择查询类型，并核对提供商 Base URL。"
	case balance.FailureInvalidResponse:
		return "请确认上游版本兼容；若使用中转服务，请检查其余额接口响应。"
	case balance.FailureAuthentication:
		return "请重新生成或更换密钥，并确认密钥与服务区域一致。"
	case balance.FailurePermission:
		return "请为账户或密钥开启余额与用量读取权限。"
	}
	return ""
}

func failureRetryable(kind string) bool {
	switch kind {
	case balance.FailureRateLimited, balance.FailureConflict, balance.FailureTimeout, balance.FailureDNS,
		balance.FailureTLS, balance.FailureNetwork, balance.FailureProxy,
		balance.FailureInvalidResponse, balance.FailureService,
		balance.FailureQuotaExhausted:
		return true
	default:
		return false
	}
}

func httpStatusFromMessage(message string) int {
	lower := strings.ToLower(message)
	for status := 400; status <= 599; status++ {
		if strings.Contains(lower, "http "+strconv.Itoa(status)) {
			return status
		}
	}
	return 0
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func isAuditedUnsupportedReason(message string) bool {
	trimmed := strings.TrimSpace(message)
	return strings.HasPrefix(trimmed, "官方未提供") ||
		strings.HasPrefix(trimmed, "小米官方未提供") ||
		strings.HasPrefix(trimmed, "小米 Token Plan") ||
		strings.HasPrefix(trimmed, "LongCat 官方未提供") ||
		strings.HasPrefix(trimmed, "OpenCode 官方未提供") ||
		strings.HasPrefix(trimmed, "火山引擎官方尚未公开")
}

func accountCacheKey(account accountQuery) string {
	secretHash := sha256.Sum256([]byte(account.APIKey))
	identity := strings.Join([]string{
		string(account.QueryType),
		strings.TrimSpace(account.BaseURL),
		strings.TrimSpace(account.ProxyURL),
		hex.EncodeToString(secretHash[:]),
	}, "\x00")
	digest := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(digest[:])
}

func htmlResponse(status int, body []byte) managementResponse {
	return managementResponse{
		StatusCode: status,
		Headers: http.Header{
			"Content-Type":            {"text/html; charset=utf-8"},
			"Cache-Control":           {"no-store"},
			"X-Content-Type-Options":  {"nosniff"},
			"Referrer-Policy":         {"no-referrer"},
			"Content-Security-Policy": {"default-src 'none'; base-uri 'none'; object-src 'none'; form-action 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data:; connect-src http: https:; frame-ancestors *"},
		},
		Body: body,
	}
}

func jsonResponse(status int, value any) managementResponse {
	body, err := json.Marshal(value)
	if err != nil {
		body = []byte(`{"error":"生成响应失败"}`)
		status = http.StatusInternalServerError
	}
	return managementResponse{
		StatusCode: status,
		Headers: http.Header{
			"Content-Type":           {"application/json; charset=utf-8"},
			"Cache-Control":          {"no-store"},
			"X-Content-Type-Options": {"nosniff"},
		},
		Body: body,
	}
}

func okEnvelope(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope{OK: true, Result: raw})
}

func errorEnvelope(code, message string) []byte {
	raw, _ := json.Marshal(envelope{OK: false, Error: &envelopeError{Code: code, Message: message}})
	return raw
}

func writeResponse(response *C.cliproxy_buffer, raw []byte) {
	if response == nil || len(raw) == 0 {
		return
	}
	ptr := C.CBytes(raw)
	if ptr == nil {
		return
	}
	response.ptr = ptr
	response.len = C.size_t(len(raw))
}
