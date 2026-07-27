// Package providers contains balance fetchers for each supported AI platform.
package providers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/Hamster-Prime/balance-query/internal/balance"
)

var directTransport = func() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return transport
}()

var httpClient = &http.Client{Timeout: 10 * time.Second, Transport: directTransport}

// getJSON performs GET with Bearer auth and decodes JSON into dest.
func getJSON(url, bearerToken, proxyURL string, dest any) error {
	if strings.TrimSpace(bearerToken) == "" {
		return fmt.Errorf("接口密钥为空")
	}
	return doGet(url, map[string]string{"Authorization": "Bearer " + bearerToken}, proxyURL, dest)
}

// getJSONRawAuth performs GET with a raw Authorization value (no "Bearer " prefix).
// Used by GLM whose API requires the token directly.
func getJSONRawAuth(url, rawToken, proxyURL string, dest any) error {
	if strings.TrimSpace(rawToken) == "" {
		return fmt.Errorf("接口密钥为空")
	}
	return doGet(url, map[string]string{"Authorization": rawToken}, proxyURL, dest)
}

// getJSONWithHeaders performs a JSON GET with provider-specific headers.
func getJSONWithHeaders(url, proxyURL string, headers map[string]string, dest any) error {
	return doGet(url, headers, proxyURL, dest)
}

func doGet(url string, headers map[string]string, proxyURL string, dest any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败：%w", err)
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
		return err
	}
	defer cleanup()

	resp, err := client.Do(req)
	if err != nil {
		if usingProxy {
			return fmt.Errorf("通过所配置代理请求余额接口失败")
		}
		return fmt.Errorf("请求余额接口失败：%w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
	if err != nil {
		return fmt.Errorf("读取余额接口响应失败：%w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("余额接口返回 HTTP %d：%s", resp.StatusCode, truncate(string(body), 200))
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("解析余额接口响应失败：%w", err)
	}
	return nil
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
	client := &http.Client{Timeout: 10 * time.Second, Transport: transport}
	return client, transport.CloseIdleConnections, true, nil
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
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func errResult(authID, provider, msg string) balance.Result {
	return balance.Result{
		Provider:  provider,
		AuthID:    authID,
		Error:     msg,
		FetchedAt: time.Now(),
	}
}
