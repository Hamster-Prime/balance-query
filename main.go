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
	pluginVersion = "0.8.0"
	abiVersion    = 1
	schemaVersion = 1

	resourcePath = "/dashboard"
	queryPath    = "/" + pluginID + "/query"

	defaultTTLSeconds = 300
	maxQueryAccounts  = 128
	maxQueryBodyBytes = 1 << 20
)

var (
	resultCache = cache.New[string, balance.Result](defaultTTLSeconds * time.Second)
	querySlots  = make(chan struct{}, 8)

	stateMu sync.RWMutex
	state   = balance.PluginConfig{
		CacheTTLSeconds:  defaultTTLSeconds,
		ProviderMappings: map[string]balance.ProviderType{},
	}
)

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
func cliproxyPluginCall(method *C.char, request *C.uint8_t, requestLen C.size_t, response *C.cliproxy_buffer) C.int {
	if response != nil {
		response.ptr = nil
		response.len = 0
	}
	if method == nil {
		writeResponse(response, errorEnvelope("invalid_method", "缺少插件调用方法"))
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
func cliproxyPluginShutdown() {}

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
	if len(request.ConfigYAML) == 0 {
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
		resultCache.SetTTL(time.Duration(next.CacheTTLSeconds) * time.Second)
		resultCache.Flush()
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
		if err := validateAccountQuery(query.Accounts[i]); err != nil {
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

func validateAccountQuery(account accountQuery) error {
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
	if !balance.IsKnownProvider(account.QueryType) {
		return fmt.Errorf("未知的余额查询类型")
	}
	if len(account.BaseURL) > 4096 {
		return fmt.Errorf("接口地址过长")
	}
	parsed, err := url.Parse(strings.TrimSpace(account.BaseURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return fmt.Errorf("接口地址必须是有效的 HTTP(S) URL")
	}
	if err := validateProxyURL(account.ProxyURL); err != nil {
		return err
	}
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
		if refresh {
			resultCache.Delete(cacheKey)
		} else if cached, ok := resultCache.Get(cacheKey); ok {
			cached.ProviderKey = account.ProviderKey
			cached.AuthID = account.ID
			cached.AccountName = account.AccountName
			cached.KeyPreview = maskAPIKey(account.APIKey)
			cached.BaseURL = account.BaseURL
			results[index] = cached
			continue
		}

		wait.Add(1)
		go func(resultIndex int, item accountQuery, key string) {
			defer wait.Done()
			querySlots <- struct{}{}
			defer func() { <-querySlots }()

			result := fetchAccount(item)
			results[resultIndex] = result
			if result.Error == "" && currentTTL() > 0 {
				resultCache.Set(key, result)
			}
		}(index, account, cacheKey)
	}
	wait.Wait()
	return results
}

func fetchAccount(account accountQuery) balance.Result {
	fetcher := providers.Build(account.QueryType, account.BaseURL)
	if fetcher == nil {
		return accountError(account, "未找到对应的余额查询器")
	}
	result := fetcher.Fetch(account.ID, account.APIKey, account.ProxyURL)
	redactResultSecret(&result, account.APIKey)
	result.ProviderKey = account.ProviderKey
	result.AuthID = account.ID
	result.AccountName = account.AccountName
	result.KeyPreview = maskAPIKey(account.APIKey)
	result.BaseURL = account.BaseURL
	return result
}

func accountError(account accountQuery, message string) balance.Result {
	return balance.Result{
		Provider:    balance.ProviderLabel[account.QueryType],
		ProviderKey: account.ProviderKey,
		AuthID:      account.ID,
		AccountName: account.AccountName,
		KeyPreview:  maskAPIKey(account.APIKey),
		BaseURL:     account.BaseURL,
		Error:       message,
		FetchedAt:   time.Now(),
	}
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
		if secret == "" {
			return value
		}
		return strings.ReplaceAll(value, secret, maskAPIKey(secret))
	}
	result.Error = localizeFetchError(redact(result.Error))
	result.QuotaDisplay = redact(result.QuotaDisplay)
	result.Plan = redact(result.Plan)
	result.ResetAt = redact(result.ResetAt)
	for key, value := range result.Extra {
		result.Extra[key] = redact(value)
	}
	for index := range result.QuotaWindows {
		window := &result.QuotaWindows[index]
		window.Group = redact(window.Group)
		window.Label = redact(window.Label)
		window.Unit = redact(window.Unit)
		window.ResetAt = redact(window.ResetAt)
		window.Status = redact(window.Status)
	}
}

func localizeFetchError(message string) string {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return ""
	}
	lower := strings.ToLower(trimmed)
	switch {
	case strings.Contains(lower, "http 401"), strings.Contains(lower, "unauthorized"), strings.Contains(lower, "invalid api key"):
		return "余额接口拒绝了密钥，请检查密钥是否有效"
	case strings.Contains(lower, "http 403"), strings.Contains(lower, "forbidden"), strings.Contains(lower, "permission"):
		return "账户没有访问余额接口的权限"
	case strings.Contains(lower, "http 404"), strings.Contains(lower, "not found"):
		return "未找到余额接口，请检查查询类型和接口地址"
	case strings.Contains(lower, "http 429"), strings.Contains(lower, "too many requests"), strings.Contains(lower, "rate limit"):
		return "余额接口请求过于频繁，请稍后重试"
	case strings.Contains(lower, "http 5"):
		return "余额服务暂时不可用，请稍后重试"
	case strings.Contains(lower, "代理"), strings.Contains(lower, "proxy"):
		return "通过代理连接余额服务失败，请检查该密钥的代理设置"
	case strings.Contains(lower, "解析余额接口响应失败"), strings.Contains(lower, "invalid character"), strings.Contains(lower, "json"):
		return "余额接口返回了无法识别的数据"
	case strings.Contains(lower, "请求余额接口失败"), strings.Contains(lower, "timeout"), strings.Contains(lower, "deadline"), strings.Contains(lower, "connection"), strings.Contains(lower, "dial tcp"), strings.Contains(lower, "no such host"), strings.Contains(lower, "tls"), strings.Contains(lower, "certificate"):
		return "无法连接余额服务，请检查网络、代理或接口地址"
	case strings.HasPrefix(trimmed, "官方未提供"),
		strings.HasPrefix(trimmed, "官方接口未返回"),
		strings.HasPrefix(trimmed, "小米官方未提供"),
		strings.HasPrefix(trimmed, "小米 Token Plan"),
		strings.HasPrefix(trimmed, "LongCat 官方未提供"),
		strings.HasPrefix(trimmed, "OpenCode 官方未提供"),
		strings.HasPrefix(trimmed, "火山引擎官方尚未公开"),
		strings.HasPrefix(trimmed, "Sub2API 未返回"),
		strings.HasPrefix(trimmed, "MiniMax 配额接口"),
		strings.HasPrefix(trimmed, "GLM 配额接口"),
		strings.HasPrefix(trimmed, "New API 返回"):
		return trimmed
	default:
		return "余额查询未成功，请检查查询类型、接口地址和账户状态"
	}
}

func accountCacheKey(account accountQuery) string {
	secretHash := sha256.Sum256([]byte(account.APIKey))
	identity := strings.Join([]string{
		account.ID,
		account.ProviderKey,
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
			"Content-Security-Policy": {"default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data:; connect-src http: https:; frame-ancestors *"},
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
