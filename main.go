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

static const cliproxy_host_api* stored_host;

static void store_host_api(const cliproxy_host_api* host) {
	stored_host = host;
}

static int call_host_api(const char* method, const uint8_t* request, size_t request_len, cliproxy_buffer* response) {
	if (stored_host == NULL || stored_host->call == NULL) {
		return 1;
	}
	return stored_host->call(stored_host->host_ctx, method, request, request_len, response);
}

static void free_host_buffer(void* ptr, size_t len) {
	if (stored_host != NULL && stored_host->free_buffer != NULL && ptr != NULL) {
		stored_host->free_buffer(ptr, len);
	}
}
*/
import "C"

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/router-for-me/CLIProxyAPI/v7/examples/plugin/balance-query/go/internal/balance"
	"github.com/router-for-me/CLIProxyAPI/v7/examples/plugin/balance-query/go/internal/cache"
	"github.com/router-for-me/CLIProxyAPI/v7/examples/plugin/balance-query/go/internal/providers"
	"github.com/router-for-me/CLIProxyAPI/v7/examples/plugin/balance-query/go/internal/ui"
)

const (
	pluginName    = "balance-query"
	pluginVersion = "1.0.0"
	abiVersion    = 1
	schemaVersion = 1
	resourcePath  = "/dashboard"
)

// ── global state ─────────────────────────────────────────────────────────────

var (
	resultCache = cache.New[string, balance.Result](300 * time.Second)
	lastFetchMu sync.RWMutex
	lastFetch   time.Time
	ttlSeconds  = 300
)

// ── envelope types ────────────────────────────────────────────────────────────

type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *envelopeError  `json:"error,omitempty"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ── CPA ABI boilerplate ───────────────────────────────────────────────────────

func main() {}

//export cliproxy_plugin_init
func cliproxy_plugin_init(host *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
	if plugin == nil {
		return 1
	}
	C.store_host_api(host)
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
		writeResponse(response, errorEnvelope("invalid_method", "method is required"))
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
func cliproxyPluginFree(ptr unsafe.Pointer, size C.size_t) {
	if ptr != nil {
		C.free(ptr)
	}
	_ = size
}

//export cliproxyPluginShutdown
func cliproxyPluginShutdown() {}

// ── method dispatch ───────────────────────────────────────────────────────────

func handleMethod(method string, request []byte) ([]byte, error) {
	switch method {
	case "plugin.register", "plugin.reconfigure":
		return handleRegister(request)
	case "management.register":
		return handleMgmtRegister()
	case "management.handle":
		return handleMgmtHandle(request)
	case "command_line.register":
		return handleCLIRegister()
	case "command_line.execute":
		return handleCLIExecute(request)
	default:
		return errorEnvelope("unknown_method", "unknown method: "+method), nil
	}
}

// ── plugin registration ───────────────────────────────────────────────────────

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
	Key         string `json:"key"`
	Type        string `json:"type"`
	Default     any    `json:"default,omitempty"`
	Description string `json:"description"`
}

type registrationCapabilities struct {
	ManagementAPI     bool `json:"management_api"`
	CommandLinePlugin bool `json:"command_line_plugin"`
}

func handleRegister(_ []byte) ([]byte, error) {
	reg := registration{
		SchemaVersion: schemaVersion,
		Metadata: pluginMetadata{
			Name:             pluginName,
			Version:          pluginVersion,
			Author:           "community",
			GitHubRepository: "https://github.com/router-for-me/CLIProxyAPI",
			Logo:             "https://raw.githubusercontent.com/router-for-me/CLIProxyAPI/main/docs/logo.png",
			ConfigFields: []configField{
				{
					Key:         "cache_ttl_seconds",
					Type:        "integer",
					Default:     300,
					Description: "Balance result cache TTL in seconds (10–86400). Set to 0 to disable caching.",
				},
			},
		},
		Capabilities: registrationCapabilities{
			ManagementAPI:     true,
			CommandLinePlugin: true,
		},
	}
	return okEnvelope(reg)
}

// ── management API ────────────────────────────────────────────────────────────

type mgmtRegistration struct {
	Resources []mgmtResource `json:"resources,omitempty"`
}

type mgmtResource struct {
	Path        string `json:"Path"`
	Menu        string `json:"Menu"`
	Description string `json:"Description"`
}

type mgmtRequest struct {
	Method  string      `json:"Method"`
	Path    string      `json:"Path"`
	Headers http.Header `json:"Headers"`
	Query   url.Values  `json:"Query"`
	Body    []byte      `json:"Body"`
}

type mgmtResponse struct {
	StatusCode int         `json:"StatusCode"`
	Headers    http.Header `json:"Headers"`
	Body       []byte      `json:"Body"`
}

func handleMgmtRegister() ([]byte, error) {
	return okEnvelope(mgmtRegistration{
		Resources: []mgmtResource{{
			Path:        resourcePath,
			Menu:        "Balance Query",
			Description: "View balance and quota status for all configured AI providers.",
		}},
	})
}

func handleMgmtHandle(raw []byte) ([]byte, error) {
	var req mgmtRequest
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, fmt.Errorf("decode management request: %w", err)
		}
	}

	action := req.Query.Get("action")

	switch action {
	case "set_ttl":
		if ttlStr := req.Query.Get("ttl"); ttlStr != "" {
			if v, err := strconv.Atoi(ttlStr); err == nil && v >= 10 && v <= 86400 {
				ttlSeconds = v
				resultCache.SetTTL(time.Duration(v) * time.Second)
			}
		}
		return okEnvelope(redirect(resourcePath))

	case "refresh":
		resultCache.Flush()
		return okEnvelope(redirect(resourcePath))

	case "settings":
		return handleSettingsPage()

	case "save_config":
		return handleSaveConfig(req)

	default:
		// Default: dashboard.
		results := fetchAll()
		lastFetchMu.RLock()
		fetchedAt := lastFetch
		lastFetchMu.RUnlock()
		page := ui.RenderDashboard(results, ttlSeconds, fetchedAt)
		return okEnvelope(htmlResp(http.StatusOK, page, nil))
	}
}

// handleSettingsPage renders the settings page.
func handleSettingsPage() ([]byte, error) {
	auths, err := listAuthEntries()
	if err != nil {
		return nil, fmt.Errorf("list auth entries: %w", err)
	}
	cfg, _ := loadPluginConfig() // best-effort; empty config on error

	uiAuths := make([]ui.AuthEntry, len(auths))
	for i, a := range auths {
		uiAuths[i] = ui.AuthEntry{
			AuthIndex: a.AuthIndex,
			Name:      a.Name,
			Provider:  a.Provider,
		}
	}
	page := ui.RenderSettings(uiAuths, cfg, "")
	return okEnvelope(htmlResp(http.StatusOK, page, nil))
}

// handleSaveConfig parses the posted form and persists the PluginConfig.
func handleSaveConfig(req mgmtRequest) ([]byte, error) {
	// The body is an application/x-www-form-urlencoded payload.
	body := string(req.Body)
	form, err := url.ParseQuery(body)
	if err != nil {
		return nil, fmt.Errorf("parse form body: %w", err)
	}

	cfg := balance.PluginConfig{
		Mappings: make(map[string]balance.AuthMapping),
	}

	for key, vals := range form {
		if !strings.HasPrefix(key, "p_") {
			continue
		}
		authIndex := strings.TrimPrefix(key, "p_")
		providerVal := ""
		if len(vals) > 0 {
			providerVal = vals[0]
		}
		if providerVal == "" {
			continue // not configured — skip
		}
		mapping := balance.AuthMapping{
			Provider: balance.ProviderType(providerVal),
		}
		// If provider needs a base URL, pull it from url_<authIndex>.
		if balance.NeedsBaseURL(mapping.Provider) {
			urlKey := "url_" + authIndex
			if uv := form.Get(urlKey); uv != "" {
				mapping.BaseURL = uv
			}
		}
		cfg.Mappings[authIndex] = mapping
	}

	if err := savePluginConfig(cfg); err != nil {
		// Re-render settings page with the error.
		auths, _ := listAuthEntries()
		uiAuths := make([]ui.AuthEntry, len(auths))
		for i, a := range auths {
			uiAuths[i] = ui.AuthEntry{
				AuthIndex: a.AuthIndex,
				Name:      a.Name,
				Provider:  a.Provider,
			}
		}
		page := ui.RenderSettings(uiAuths, cfg, err.Error())
		return okEnvelope(htmlResp(http.StatusOK, page, nil))
	}

	// Flush cache so next dashboard load uses new config.
	resultCache.Flush()
	return okEnvelope(redirect(resourcePath))
}

func redirect(location string) mgmtResponse {
	return mgmtResponse{
		StatusCode: http.StatusFound,
		Headers:    http.Header{"Location": {location}},
		Body:       []byte("Redirecting…"),
	}
}

func htmlResp(status int, body []byte, extra map[string][]string) mgmtResponse {
	headers := http.Header{"content-type": {"text/html; charset=utf-8"}}
	for k, v := range extra {
		headers[k] = v
	}
	return mgmtResponse{StatusCode: status, Headers: headers, Body: body}
}

// ── CLI ───────────────────────────────────────────────────────────────────────

type cliFlagDef struct {
	Name  string `json:"Name"`
	Usage string `json:"Usage"`
	Type  string `json:"Type"`
}

type cliRegisterResp struct {
	Flags []cliFlagDef `json:"Flags"`
}

type cliExecuteResp struct {
	Stdout   string `json:"Stdout"` // base64-encoded
	ExitCode int    `json:"ExitCode"`
}

func handleCLIRegister() ([]byte, error) {
	return okEnvelope(cliRegisterResp{
		Flags: []cliFlagDef{
			{Name: "balance", Usage: "Query balance and quota for all configured providers", Type: "bool"},
			{Name: "balance-refresh", Usage: "Force-refresh cached balance data", Type: "bool"},
		},
	})
}

func handleCLIExecute(raw []byte) ([]byte, error) {
	var flags map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &flags)
	}
	if v, ok := flags["balance-refresh"]; ok && v == true {
		resultCache.Flush()
	}
	results := fetchAll()
	text := ui.RenderCLITable(results)
	return okEnvelope(cliExecuteResp{
		Stdout:   base64.StdEncoding.EncodeToString([]byte(text)),
		ExitCode: 0,
	})
}

// ── core fetch logic ──────────────────────────────────────────────────────────

// hostAuthListResp mirrors the CPA host.auth.list response.
type hostAuthListResp struct {
	Files []hostAuthEntry `json:"files"`
}

type hostAuthEntry struct {
	AuthIndex string `json:"auth_index"`
	Name      string `json:"name"`
	Provider  string `json:"provider"`
}

// hostAuthGetRuntimeResp mirrors the CPA host.auth.get_runtime response.
type hostAuthGetRuntimeResp struct {
	Token string `json:"token"`
}

// listAuthEntries fetches the full auth list from the host.
func listAuthEntries() ([]hostAuthEntry, error) {
	raw, err := callHost("host.auth.list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var resp hostAuthListResp
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode auth list: %w", err)
	}
	return resp.Files, nil
}

// fetchAll retrieves balance results for all CPA auth entries,
// using the manual provider mapping and the in-memory cache.
func fetchAll() []balance.Result {
	auths, err := listAuthEntries()
	if err != nil {
		return []balance.Result{{
			Provider:  "system",
			Error:     "host.auth.list failed: " + err.Error(),
			FetchedAt: time.Now(),
		}}
	}

	cfg, _ := loadPluginConfig() // best-effort; empty on error

	results := make([]balance.Result, len(auths))
	var wg sync.WaitGroup
	for i, entry := range auths {
		if cached, ok := resultCache.Get(entry.AuthIndex); ok {
			results[i] = cached
			continue
		}
		wg.Add(1)
		go func(idx int, e hostAuthEntry) {
			defer wg.Done()
			results[idx] = fetchOne(e, cfg)
		}(i, entry)
	}
	wg.Wait()

	lastFetchMu.Lock()
	lastFetch = time.Now()
	lastFetchMu.Unlock()

	return results
}

// fetchOne fetches the balance for a single auth entry using its manual mapping.
func fetchOne(entry hostAuthEntry, cfg balance.PluginConfig) balance.Result {
	mapping, ok := cfg.Mappings[entry.AuthIndex]
	if !ok || mapping.Provider == "" {
		return balance.Result{
			Provider:  entry.Provider,
			AuthID:    entry.AuthIndex,
			Error:     "not configured — assign a provider type in Settings",
			FetchedAt: time.Now(),
		}
	}

	fetcher := providers.Build(mapping.Provider, mapping.BaseURL)
	if fetcher == nil {
		return balance.Result{
			Provider:  string(mapping.Provider),
			AuthID:    entry.AuthIndex,
			Error:     "unknown provider type: " + string(mapping.Provider),
			FetchedAt: time.Now(),
		}
	}

	tokenRaw, err := callHost("host.auth.get_runtime", map[string]any{"auth_index": entry.AuthIndex})
	if err != nil {
		return balance.Result{
			Provider:  string(mapping.Provider),
			AuthID:    entry.AuthIndex,
			Error:     "get_runtime failed: " + err.Error(),
			FetchedAt: time.Now(),
		}
	}
	var rtResp hostAuthGetRuntimeResp
	if err := json.Unmarshal(tokenRaw, &rtResp); err != nil {
		return balance.Result{
			Provider:  string(mapping.Provider),
			AuthID:    entry.AuthIndex,
			Error:     "decode runtime token: " + err.Error(),
			FetchedAt: time.Now(),
		}
	}

	result := fetcher.Fetch(entry.AuthIndex, rtResp.Token)
	resultCache.Set(entry.AuthIndex, result)
	return result
}

// ── plugin config persistence ─────────────────────────────────────────────────

// loadPluginConfig reads the PluginConfig from the host.auth.get store.
func loadPluginConfig() (balance.PluginConfig, error) {
	raw, err := callHost("host.auth.get", map[string]any{"name": balance.ConfigFileName})
	if err != nil {
		// File not found yet — return empty config.
		return balance.PluginConfig{Mappings: map[string]balance.AuthMapping{}}, nil
	}
	// host.auth.get returns the file content as a base64-encoded string or raw bytes.
	// Attempt direct JSON unmarshal first; fall back to base64 decoding.
	var cfg balance.PluginConfig
	if err2 := json.Unmarshal(raw, &cfg); err2 == nil {
		if cfg.Mappings == nil {
			cfg.Mappings = map[string]balance.AuthMapping{}
		}
		return cfg, nil
	}
	// The result might be a JSON string containing the raw file bytes as base64.
	var b64str string
	if err3 := json.Unmarshal(raw, &b64str); err3 == nil {
		decoded, err4 := base64.StdEncoding.DecodeString(b64str)
		if err4 == nil {
			if err5 := json.Unmarshal(decoded, &cfg); err5 == nil {
				if cfg.Mappings == nil {
					cfg.Mappings = map[string]balance.AuthMapping{}
				}
				return cfg, nil
			}
		}
	}
	return balance.PluginConfig{Mappings: map[string]balance.AuthMapping{}}, nil
}

// savePluginConfig persists PluginConfig via host.auth.save.
func savePluginConfig(cfg balance.PluginConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	_, err = callHost("host.auth.save", map[string]any{
		"name":    balance.ConfigFileName,
		"content": string(data),
	})
	return err
}

// ── host callback helper ──────────────────────────────────────────────────────

func callHost(method string, payload any) (json.RawMessage, error) {
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal %s payload: %w", method, err)
	}
	cMethod := C.CString(method)
	defer C.free(unsafe.Pointer(cMethod))

	var response C.cliproxy_buffer
	var requestPtr *C.uint8_t
	if len(rawPayload) > 0 {
		cPayload := C.CBytes(rawPayload)
		if cPayload == nil {
			return nil, fmt.Errorf("allocate payload for %s", method)
		}
		defer C.free(cPayload)
		requestPtr = (*C.uint8_t)(cPayload)
	}

	callCode := C.call_host_api(cMethod, requestPtr, C.size_t(len(rawPayload)), &response)
	var rawResponse []byte
	if response.ptr != nil && response.len > 0 {
		rawResponse = C.GoBytes(response.ptr, C.int(response.len))
	}
	if response.ptr != nil {
		C.free_host_buffer(response.ptr, response.len)
	}
	if len(rawResponse) == 0 {
		return nil, fmt.Errorf("host %s returned no response (code=%d)", method, int(callCode))
	}

	var env envelope
	if err := json.Unmarshal(rawResponse, &env); err != nil {
		return nil, fmt.Errorf("decode host %s envelope: %w", method, err)
	}
	if !env.OK {
		if env.Error != nil {
			return nil, fmt.Errorf("%s: %s", env.Error.Code, env.Error.Message)
		}
		return nil, fmt.Errorf("host %s failed", method)
	}
	if callCode != 0 {
		return nil, fmt.Errorf("host %s returned code=%d", method, int(callCode))
	}
	return append(json.RawMessage(nil), env.Result...), nil
}

// ── envelope helpers ──────────────────────────────────────────────────────────

func okEnvelope(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
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
