// Package providers contains balance fetchers for each supported AI platform.
package providers

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Hamster-Prime/balance-query/internal/balance"
)

var directTransport = func() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return transport
}()

var httpClient = &http.Client{
	Timeout:       10 * time.Second,
	Transport:     directTransport,
	CheckRedirect: providerRedirectPolicy,
}

const maxProviderResponseBytes int64 = 2 << 20

// ProviderError is the structured, provider-safe error returned by the shared
// HTTP layer. It deliberately keeps untrusted response bodies out of Error()
// while preserving the fields callers need for stable failure classification.
type ProviderError struct {
	Kind              string
	Message           string
	HTTPStatus        int
	ProviderCode      string
	RequestID         string
	RetryAfterSeconds int64
	Cause             error
}

func (err *ProviderError) Error() string {
	if err == nil {
		return ""
	}
	return err.Message
}

func (err *ProviderError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

// getJSON performs GET with Bearer auth and decodes JSON into dest.
func getJSON(url, bearerToken, proxyURL string, dest any) error {
	if strings.TrimSpace(bearerToken) == "" {
		return newProviderError(balance.FailureAuthentication, "接口密钥为空", 0, "", nil)
	}
	return doGet(url, map[string]string{"Authorization": "Bearer " + bearerToken}, proxyURL, dest)
}

// getJSONRawAuth performs GET with a raw Authorization value (no "Bearer " prefix).
// Used by GLM whose API requires the token directly.
func getJSONRawAuth(url, rawToken, proxyURL string, dest any) error {
	if strings.TrimSpace(rawToken) == "" {
		return newProviderError(balance.FailureAuthentication, "接口密钥为空", 0, "", nil)
	}
	return doGet(url, map[string]string{"Authorization": rawToken}, proxyURL, dest)
}

// getJSONWithHeaders performs a JSON GET with provider-specific headers.
func getJSONWithHeaders(url, proxyURL string, headers map[string]string, dest any) error {
	return doGet(url, headers, proxyURL, dest)
}

func doGet(url string, headers map[string]string, proxyURL string, dest any) error {
	return doGetContext(context.Background(), url, headers, proxyURL, dest)
}

func getJSONWithHeadersContext(ctx context.Context, url, proxyURL string, headers map[string]string, dest any) error {
	return doGetContext(ctx, url, headers, proxyURL, dest)
}

func doGetContext(ctx context.Context, url string, headers map[string]string, proxyURL string, dest any) error {
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return newProviderError(balance.FailureInvalidConfig, "创建请求失败", 0, "", err)
	}
	for key, value := range headers {
		if strings.TrimSpace(value) != "" {
			req.Header.Set(key, value)
		}
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	client, cleanup, usingProxy, err := clientForProxy(proxyURL)
	if err != nil {
		return newProviderError(balance.FailureProxy, safeProviderMessage(err.Error()), 0, "", err)
	}
	defer cleanup()

	resp, err := client.Do(req)
	if err != nil {
		return transportProviderError(err, usingProxy)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxProviderResponseBytes+1))
	oversized := int64(len(body)) > maxProviderResponseBytes
	if resp.StatusCode >= 300 {
		secrets := providerCredentialValues(headers)
		message, code, bodyRequestID := "", "", ""
		if !oversized {
			message, code, bodyRequestID = parseProviderErrorBody(body, secrets...)
		}
		requestID := firstNonEmpty(
			resp.Header.Get("request-id"),
			resp.Header.Get("x-request-id"),
			resp.Header.Get("anthropic-request-id"),
			bodyRequestID,
		)
		display := fmt.Sprintf("余额接口返回 HTTP %d", resp.StatusCode)
		if message != "" {
			display += "：" + message
		}
		return &ProviderError{
			Kind:              classifyProviderFailure(resp.StatusCode, code, message),
			Message:           display,
			HTTPStatus:        resp.StatusCode,
			ProviderCode:      code,
			RequestID:         safeProviderMessageWithSecrets(requestID, secrets),
			RetryAfterSeconds: parseRetryAfter(resp.Header.Get("Retry-After"), time.Now()),
			Cause:             readErr,
		}
	}
	if readErr != nil {
		failure := transportProviderError(readErr, usingProxy).(*ProviderError)
		failure.HTTPStatus = resp.StatusCode
		failure.Message = "读取余额接口响应失败：" + failure.Message
		return failure
	}
	if oversized {
		return newProviderError(balance.FailureInvalidResponse, "余额接口响应超过 2 MiB 安全上限", resp.StatusCode, "", nil)
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return newProviderError(balance.FailureInvalidResponse, "解析余额接口响应失败", resp.StatusCode, "", err)
	}
	return nil
}

func transportProviderError(err error, usingProxy bool) error {
	kind := balance.FailureNetwork
	message := "请求余额接口失败"
	if errors.Is(err, context.DeadlineExceeded) {
		kind = balance.FailureTimeout
		message = "请求余额接口超时"
	} else {
		var dnsErr *net.DNSError
		var netErr net.Error
		var certificateVerificationErr *tls.CertificateVerificationError
		var unknownAuthority x509.UnknownAuthorityError
		var hostnameErr x509.HostnameError
		lowerMessage := strings.ToLower(err.Error())
		switch {
		case errors.As(err, &netErr) && netErr.Timeout():
			kind = balance.FailureTimeout
			message = "请求余额接口超时"
		case errors.As(err, &certificateVerificationErr), errors.As(err, &unknownAuthority), errors.As(err, &hostnameErr),
			strings.Contains(lowerMessage, "tls"), strings.Contains(lowerMessage, "x509:"), strings.Contains(lowerMessage, "certificate"):
			kind = balance.FailureTLS
			message = "余额接口 TLS 校验失败"
		case strings.Contains(lowerMessage, "redirect"), strings.Contains(lowerMessage, "重定向"):
			kind = balance.FailureEndpoint
			message = "余额接口重定向无效"
		case usingProxy:
			kind = balance.FailureProxy
			message = "通过所配置代理请求余额接口失败"
		case errors.As(err, &dnsErr):
			kind = balance.FailureDNS
			message = "无法解析余额接口域名"
		}
	}
	return newProviderError(kind, message, 0, "", err)
}

func parseProviderErrorBody(body []byte, secrets ...string) (message, code, requestID string) {
	var payload map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return "", "", ""
	}

	if nested, ok := payload["error"].(map[string]any); ok {
		message = firstSafeMapString(nested, secrets, "message", "detail", "reason")
		code = firstSafeMapScalar(nested, secrets, "code", "type")
		requestID = firstSafeMapString(nested, secrets, "request_id", "requestId")
	} else if text, ok := payload["error"].(string); ok {
		message = safeProviderMessageWithSecrets(text, secrets)
	}
	if message == "" {
		message = firstSafeMapString(payload, secrets, "message", "msg", "detail", "reason", "status_msg")
	}
	if code == "" {
		code = firstSafeMapScalar(payload, secrets, "code", "error_code", "scode", "type")
	}
	if requestID == "" {
		requestID = firstSafeMapString(payload, secrets, "request_id", "requestId")
	}
	if baseResp, ok := payload["base_resp"].(map[string]any); ok {
		if message == "" {
			message = firstSafeMapString(baseResp, secrets, "status_msg", "message")
		}
		if code == "" {
			code = firstSafeMapScalar(baseResp, secrets, "status_code", "code")
		}
	}
	if data, ok := payload["data"].(map[string]any); ok {
		if nested, ok := data["error"].(map[string]any); ok {
			if message == "" {
				message = firstSafeMapString(nested, secrets, "message", "detail", "reason")
			}
			if code == "" {
				code = firstSafeMapScalar(nested, secrets, "code", "type")
			}
			if requestID == "" {
				requestID = firstSafeMapString(nested, secrets, "request_id", "requestId")
			}
		} else if text, ok := data["error"].(string); ok && message == "" {
			message = safeProviderMessageWithSecrets(text, secrets)
		}
		if message == "" {
			message = firstSafeMapString(data, secrets, "message", "msg", "detail", "reason", "status_msg")
		}
		if code == "" {
			code = firstSafeMapScalar(data, secrets, "code", "error_code", "scode", "type")
		}
		if baseResp, ok := data["base_resp"].(map[string]any); ok {
			if message == "" {
				message = firstSafeMapString(baseResp, secrets, "status_msg", "message")
			}
			if code == "" {
				code = firstSafeMapScalar(baseResp, secrets, "status_code", "code")
			}
		}
	}
	return safeProviderMessageWithSecrets(message, secrets), safeProviderMessageWithSecrets(code, secrets), safeProviderMessageWithSecrets(requestID, secrets)
}

func firstSafeMapString(values map[string]any, secrets []string, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key].(string); ok {
			if safe := safeProviderMessageWithSecrets(value, secrets); safe != "" {
				return safe
			}
		}
	}
	return ""
}

func firstSafeMapScalar(values map[string]any, secrets []string, keys ...string) string {
	for _, key := range keys {
		switch value := values[key].(type) {
		case string:
			if safe := safeProviderMessageWithSecrets(value, secrets); safe != "" {
				return safe
			}
		case json.Number:
			return value.String()
		case float64:
			return strconv.FormatFloat(value, 'f', -1, 64)
		}
	}
	return ""
}

func providerCredentialValues(headers map[string]string) []string {
	secrets := make([]string, 0, len(headers)*2)
	for key, value := range headers {
		lowerKey := strings.ToLower(strings.TrimSpace(key))
		if !strings.Contains(lowerKey, "authorization") && !strings.Contains(lowerKey, "api-key") && !strings.Contains(lowerKey, "api_key") && !strings.Contains(lowerKey, "token") {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		secrets = append(secrets, value)
		if fields := strings.Fields(value); len(fields) == 2 {
			secrets = append(secrets, fields[1])
		}
	}
	return secrets
}

func safeProviderMessageWithSecrets(value string, secrets []string) string {
	for _, secret := range secrets {
		if secret = strings.TrimSpace(secret); len(secret) >= 4 {
			value = strings.ReplaceAll(value, secret, "[已隐藏]")
		}
	}
	return safeProviderMessage(value)
}

func safeProviderMessage(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && !unicode.IsSpace(r) {
			return -1
		}
		return r
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	return truncate(value, 240)
}

func parseRetryAfter(value string, now time.Time) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if seconds > 0 {
			return seconds
		}
		return 0
	}
	if retryAt, err := http.ParseTime(value); err == nil {
		if now.IsZero() {
			now = time.Now()
		}
		delay := retryAt.Sub(now)
		if delay > 0 {
			seconds := int64(delay / time.Second)
			if delay%time.Second != 0 {
				seconds++
			}
			return seconds
		}
	}
	return 0
}

func newProviderError(kind, message string, status int, code string, cause error) *ProviderError {
	if kind == "" {
		kind = classifyProviderFailure(status, code, message)
	}
	return &ProviderError{
		Kind:         kind,
		Message:      safeProviderMessage(message),
		HTTPStatus:   status,
		ProviderCode: safeProviderMessage(code),
		Cause:        cause,
	}
}

func providerBusinessError(code, message string, secrets ...string) *ProviderError {
	code = safeProviderMessageWithSecrets(code, secrets)
	message = safeProviderMessageWithSecrets(message, secrets)
	return newProviderError(classifyProviderFailure(0, code, message), message, 0, code, nil)
}

func invalidResponseError(message string) *ProviderError {
	return newProviderError(balance.FailureInvalidResponse, message, 0, "", nil)
}

func isAuthenticationFailure(err error) bool {
	var providerErr *ProviderError
	return errors.As(err, &providerErr) && providerErr.Kind == balance.FailureAuthentication
}

func classifyProviderFailure(status int, code, message string) string {
	lowerCode := strings.ToLower(strings.TrimSpace(code))
	lowerMessage := strings.ToLower(strings.TrimSpace(message))
	combined := lowerCode + " " + lowerMessage
	if status >= 300 && status < 400 {
		return balance.FailureEndpoint
	}

	switch lowerCode {
	case "401", "api_key_required", "api_key_invalid", "invalid_api_key", "invalid_key", "invalid_token", "token_invalid", "unauthenticated", "unauthorized", "invalid_authentication_error", "incorrect_api_key_error", "authentication_error", "auth_unauthorized", "1004", "2049":
		return balance.FailureAuthentication
	case "api_key_disabled", "api_key_expired", "expired_api_key", "key_disabled", "account_disabled", "account_suspended", "user_not_found":
		return balance.FailureAccount
	case "403", "access_denied", "forbidden", "insufficient_permissions", "permission_denied", "permission_denied_error":
		return balance.FailurePermission
	case "insufficient_balance", "insufficient_funds", "balance_insufficient":
		return balance.FailureInsufficientFund
	case "quota_exceeded", "quota_exhausted", "usage_limit_exceeded":
		return balance.FailureQuotaExhausted
	case "429", "1002", "rate_limit_exceeded", "rate_limited", "too_many_requests":
		return balance.FailureRateLimited
	case "409", "conflict", "request_conflict":
		return balance.FailureConflict
	case "api_key_auth_overloaded", "engine_overloaded_error", "server_unavailable":
		return balance.FailureService
	}

	switch status {
	case http.StatusBadRequest, http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity:
		return balance.FailureInvalidConfig
	case http.StatusUnauthorized:
		return balance.FailureAuthentication
	case http.StatusPaymentRequired:
		return balance.FailureInsufficientFund
	case http.StatusForbidden:
		return balance.FailurePermission
	case http.StatusConflict:
		return balance.FailureConflict
	case http.StatusNotFound, http.StatusMethodNotAllowed:
		return balance.FailureEndpoint
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return balance.FailureTimeout
	case http.StatusTooManyRequests:
		if strings.Contains(combined, "quota") || strings.Contains(combined, "余额") || strings.Contains(combined, "balance") || strings.Contains(combined, "额度") || strings.Contains(combined, "usage limit") {
			return balance.FailureQuotaExhausted
		}
		if strings.Contains(combined, "overload") {
			return balance.FailureService
		}
		return balance.FailureRateLimited
	}
	if status >= 500 {
		return balance.FailureService
	}

	switch {
	case strings.Contains(combined, "invalid api key"), strings.Contains(combined, "invalid_api_key"), strings.Contains(combined, "incorrect api key"), strings.Contains(combined, "incorrect_api_key"), strings.Contains(combined, "authentication"), strings.Contains(combined, "unauthorized"), strings.Contains(combined, "密钥为空"), strings.Contains(combined, "key 无效"):
		return balance.FailureAuthentication
	case strings.Contains(combined, "permission"), strings.Contains(combined, "access denied"), strings.Contains(combined, "无权"), strings.Contains(combined, "权限"):
		return balance.FailurePermission
	case strings.Contains(combined, "insufficient balance"), strings.Contains(combined, "余额不足"), strings.Contains(combined, "欠费"), strings.Contains(combined, "exceeded_current_quota"):
		return balance.FailureInsufficientFund
	case strings.Contains(combined, "quota_exhausted"), strings.Contains(combined, "quota exhausted"), strings.Contains(combined, "额度已用尽"), strings.Contains(combined, "usage limit"):
		return balance.FailureQuotaExhausted
	case strings.Contains(combined, "rate limit"), strings.Contains(combined, "too many"), strings.Contains(combined, "限流"):
		return balance.FailureRateLimited
	case strings.Contains(combined, "timeout"), strings.Contains(combined, "timed out"), strings.Contains(combined, "超时"):
		return balance.FailureTimeout
	case strings.Contains(combined, "proxy"), strings.Contains(combined, "代理"):
		return balance.FailureProxy
	case strings.Contains(combined, "dns"), strings.Contains(combined, "解析") && strings.Contains(combined, "域名"):
		return balance.FailureDNS
	case strings.Contains(combined, "tls"), strings.Contains(combined, "certificate"):
		return balance.FailureTLS
	case strings.Contains(combined, "未提供") && strings.Contains(combined, "接口"):
		return balance.FailureUnsupported
	case strings.Contains(combined, "接口地址"), strings.Contains(combined, "配置") && (strings.Contains(combined, "无效") || strings.Contains(combined, "没有") || strings.Contains(combined, "缺少")):
		return balance.FailureInvalidConfig
	case strings.Contains(combined, "未返回"), strings.Contains(combined, "响应"), strings.Contains(combined, "解析"):
		return balance.FailureInvalidResponse
	case strings.Contains(combined, "overload"), strings.Contains(combined, "service unavailable"), strings.Contains(combined, "服务不可用"):
		return balance.FailureService
	default:
		return balance.FailureUnknown
	}
}

func clientForProxy(proxyURL string) (*http.Client, func(), bool, error) {
	trimmed := strings.TrimSpace(proxyURL)
	if trimmed == "" || strings.EqualFold(trimmed, "direct") || strings.EqualFold(trimmed, "none") {
		return httpClient, func() {}, false, nil
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return nil, nil, true, fmt.Errorf("代理地址无效")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "socks5", "socks5h":
	default:
		return nil, nil, true, fmt.Errorf("代理地址协议不受支持")
	}
	if strings.EqualFold(parsed.Scheme, "socks5h") {
		// Go 1.22's SOCKS transport uses remote hostname resolution, which is
		// the behavior CPA exposes as socks5h, but the accepted scheme is socks5.
		parsed.Scheme = "socks5"
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(parsed)
	client := &http.Client{Timeout: 10 * time.Second, Transport: transport, CheckRedirect: providerRedirectPolicy}
	return client, transport.CloseIdleConnections, true, nil
}

func providerRedirectPolicy(request *http.Request, via []*http.Request) error {
	if len(via) == 0 {
		return nil
	}
	if len(via) >= 10 {
		return errors.New("重定向次数超过安全上限")
	}
	original := via[0].URL
	if !strings.EqualFold(request.URL.Scheme, original.Scheme) || !strings.EqualFold(request.URL.Host, original.Host) {
		return http.ErrUseLastResponse
	}
	return nil
}

// serviceEndpoint derives a non-OpenAI endpoint from a CPA OpenAI-compatible
// base URL. Providers commonly configure .../v1 in CPA while their account
// endpoint lives beside that prefix (for example /api/user/self).
func serviceEndpoint(baseURL, endpoint string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("接口地址无效")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("接口地址仅支持 HTTP(S)")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("接口地址不能包含用户信息")
	}

	basePath := strings.TrimSuffix(parsed.Path, "/")
	if strings.HasSuffix(strings.ToLower(basePath), "/v1") {
		basePath = strings.TrimSuffix(basePath, basePath[len(basePath)-3:])
	}
	keepTrailingSlash := strings.HasSuffix(endpoint, "/")
	parsed.Path = path.Join(basePath, "/"+strings.TrimPrefix(endpoint, "/"))
	if keepTrailingSlash && !strings.HasSuffix(parsed.Path, "/") {
		parsed.Path += "/"
	}
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}

func errResult(authID, provider string, failure any) balance.Result {
	err := normalizeProviderError(failure)
	message := "查询失败"
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		message = safeProviderMessage(err.Error())
	}
	return balance.Result{
		Provider:  provider,
		AuthID:    authID,
		Error:     message,
		Failure:   failureInfoFromError(err, message),
		FetchedAt: time.Now(),
	}
}

func normalizeProviderError(value any) error {
	switch typed := value.(type) {
	case nil:
		return nil
	case error:
		return typed
	case string:
		return newProviderError(classifyProviderFailure(0, "", typed), typed, 0, "", nil)
	default:
		message := safeProviderMessage(fmt.Sprint(typed))
		return newProviderError(classifyProviderFailure(0, "", message), message, 0, "", nil)
	}
}

func failureInfoFromError(err error, fallbackMessage string) *balance.FailureInfo {
	kind := classifyProviderFailure(0, "", fallbackMessage)
	status := 0
	code := ""
	requestID := ""
	retryAfter := int64(0)
	var providerErr *ProviderError
	if errors.As(err, &providerErr) {
		kind = providerErr.Kind
		status = providerErr.HTTPStatus
		code = providerErr.ProviderCode
		requestID = providerErr.RequestID
		retryAfter = providerErr.RetryAfterSeconds
	}
	if kind == "" {
		kind = balance.FailureUnknown
	}
	title, suggestion, retryable := failurePresentation(kind)
	if retryAfter > 0 {
		suggestion = fmt.Sprintf("请在 %d 秒后重试", retryAfter)
		retryable = true
	}
	return &balance.FailureInfo{
		Kind:              kind,
		Title:             title,
		Reason:            fallbackMessage,
		Suggestion:        suggestion,
		Retryable:         retryable,
		HTTPStatus:        status,
		ProviderCode:      code,
		RequestID:         requestID,
		RetryAfterSeconds: retryAfter,
	}
}

func failurePresentation(kind string) (title, suggestion string, retryable bool) {
	switch kind {
	case balance.FailureAuthentication:
		return "认证失败", "请检查密钥是否正确、有效，并确认所选区域与密钥所属平台一致", false
	case balance.FailurePermission:
		return "权限不足", "请检查密钥权限、组织权限或 IP 白名单设置", false
	case balance.FailureInsufficientFund:
		return "余额不足", "请充值或检查账户账单状态", false
	case balance.FailureQuotaExhausted:
		return "额度已用尽", "请等待额度重置、升级套餐或补充可用额度", false
	case balance.FailureRateLimited:
		return "请求过于频繁", "请稍后重试并降低查询频率", true
	case balance.FailureConflict:
		return "请求冲突", "请稍后刷新；若持续出现，请检查同一密钥是否正在执行冲突操作", true
	case balance.FailureInvalidConfig:
		return "查询配置无效", "请检查服务地址、查询类型和代理配置", false
	case balance.FailureEndpoint:
		return "查询接口不可用", "请确认服务版本支持该查询接口", false
	case balance.FailureProxy:
		return "代理连接失败", "请检查代理地址、认证信息和网络连通性", true
	case balance.FailureTimeout:
		return "查询超时", "请稍后重试或检查网络与代理", true
	case balance.FailureDNS:
		return "域名解析失败", "请检查网络 DNS 或代理设置", true
	case balance.FailureTLS:
		return "安全连接失败", "请检查系统时间、证书链或 HTTPS 代理", false
	case balance.FailureNetwork:
		return "网络连接失败", "请检查网络与代理后重试", true
	case balance.FailureInvalidResponse:
		return "接口响应无法识别", "服务可能升级了响应格式，请稍后重试或更新插件", true
	case balance.FailureService:
		return "服务暂不可用", "上游服务可能过载或维护中，请稍后重试", true
	case balance.FailureAccount:
		return "账户不可用", "请检查密钥、账户或套餐状态", false
	case balance.FailureNoData:
		return "暂无可用数据", "接口未返回可展示的余额或额度数据", false
	case balance.FailureUnsupported:
		return "不支持自动查询", "请前往提供商控制台查看", false
	default:
		return "查询失败", "请检查配置后重试", false
	}
}

func providerWarning(section string, failure any) balance.FailureInfo {
	err := normalizeProviderError(failure)
	message := "查询失败"
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		message = safeProviderMessage(err.Error())
	}
	warning := failureInfoFromError(err, message)
	if section = strings.TrimSpace(section); section != "" {
		warning.Title = section + "：" + warning.Title
	}
	return *warning
}

// CloseIdleConnections releases keep-alive connections owned by the provider
// package. It is safe to call repeatedly during plugin shutdown.
func CloseIdleConnections() {
	if httpClient != nil {
		httpClient.CloseIdleConnections()
	}
	if directTransport != nil {
		directTransport.CloseIdleConnections()
	}
}
