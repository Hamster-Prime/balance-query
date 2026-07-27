package ui

import (
	"strings"
	"testing"
)

func TestRenderDashboardUsesOpenAICompatibilityManagementAPIs(t *testing.T) {
	page := string(RenderDashboard(300))
	for _, want := range []string{
		`/openai-compatibility`,
		`/plugins/balance-query/config`,
		`/proxy-url`,
		`/balance-query/query`,
		`api-key-entries`,
		`proxy-url`,
		`provider_mappings`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("dashboard is missing %q", want)
		}
	}
	for _, unwanted := range []string{
		`host.auth`,
		`balance-query-config.json`,
		`auth-files`,
	} {
		if strings.Contains(page, unwanted) {
			t.Fatalf("dashboard still references obsolete auth-file source %q", unwanted)
		}
	}
}

func TestRenderDashboardIncludesChineseThemeAwareUI(t *testing.T) {
	page := string(RenderDashboard(180))
	for _, want := range []string{
		`lang="zh-CN"`,
		`余额与配额`,
		`OpenAI 兼容提供商`,
		`查询设置`,
		`接口密钥`,
		`data-theme`,
		`MutationObserver`,
		`prefers-color-scheme: dark`,
		`prefers-reduced-motion:reduce`,
		`transition:background 150ms ease`,
		`animation:item-in 300ms ease-out`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("dashboard is missing localized theme or motion marker %q", want)
		}
	}
	if !strings.Contains(page, `var INITIAL_TTL = 180;`) {
		t.Fatal("dashboard did not embed the configured cache TTL")
	}
}

func TestRenderDashboardDoesNotInterpolateCredentials(t *testing.T) {
	page := string(RenderDashboard(300))
	for _, dangerous := range []string{
		`innerHTML`,
		`document.write`,
		`localStorage.setItem(AUTH_STORAGE_KEY`,
	} {
		if strings.Contains(page, dangerous) {
			t.Fatalf("dashboard contains unsafe credential handling marker %q", dangerous)
		}
	}
	for _, want := range []string{`textContent`, `redactSecrets`, `maskKey`} {
		if !strings.Contains(page, want) {
			t.Fatalf("dashboard is missing credential-safe rendering marker %q", want)
		}
	}
}
