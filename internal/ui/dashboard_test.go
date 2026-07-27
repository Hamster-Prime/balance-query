package ui

import (
	"regexp"
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
		`dataset.resolvedTheme`,
		`MutationObserver`,
		`window.parent.getComputedStyle`,
		`window.parent.document.body`,
		`balance-query:theme-request`,
		`cli-proxy-theme`,
		`prefers-color-scheme: dark`,
		`prefers-reduced-motion:reduce`,
		`--motion-fast:150ms ease`,
		`--motion-normal:300ms ease`,
		`animation:item-in 400ms ease-out`,
		`translate3d(0,28px,0)`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("dashboard is missing localized theme or motion marker %q", want)
		}
	}
	if !strings.Contains(page, `var INITIAL_TTL = 180;`) {
		t.Fatal("dashboard did not embed the configured cache TTL")
	}
}

func TestRenderDashboardShowsAllStructuredQuotaWindows(t *testing.T) {
	page := string(RenderDashboard(300))
	for _, want := range []string{
		`result.quota_windows`,
		`quota-groups`,
		`quota-window-grid`,
		`quota-window-value`,
		`used_percent`,
		`remaining_percent`,
		`reset_in_seconds`,
		`data-reset-at`,
		`不限量`,
		`日配额`,
		`周配额`,
		`月配额`,
		`小时配额`,
		`后重置`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("dashboard is missing structured quota UI marker %q", want)
		}
	}
}

func TestRenderDashboardShowsEveryExtraDetailInStableOrder(t *testing.T) {
	page := string(RenderDashboard(300))
	if strings.Contains(page, `Object.keys(result.extra).slice`) {
		t.Fatal("dashboard still truncates provider details")
	}
	for _, want := range []string{
		`Object.keys(result.extra).sort`,
		`detail-grid`,
		`账户明细`,
		`今日请求数`,
		`累计令牌数`,
		`每分钟请求数`,
		`加量包余额`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("dashboard is missing complete detail marker %q", want)
		}
	}
}

func TestRenderDashboardFailureCardsStayCompactWithoutDetailContent(t *testing.T) {
	page := string(RenderDashboard(300))
	for name, pattern := range map[string]string{
		`flat overview row`:        `(?s)\.result-overview\{[^}]*display:flex[^}]*flex-wrap:wrap[^}]*margin-top:`,
		`marginless primary value`: `(?s)\.quota-main\{[^}]*margin:0`,
		`no failed detail toggle`:  `var\s+detailKeys\s*=\s*!result\.error\s*\?\s*extraDetailKeys\(result\)\s*:\s*\[\]`,
	} {
		if !regexp.MustCompile(pattern).MatchString(page) {
			t.Fatalf("dashboard is missing compact failed-card behavior %s (%s)", name, pattern)
		}
	}

	failedBranch := regexp.MustCompile(`(?s)if\s*\(failed\s*\|\|\s*consoleOnly\).*?\}\s*else\s*\{.*?renderQuotaGroups`)
	if !failedBranch.MatchString(page) {
		t.Fatal("dashboard should keep quota groups out of the failed/console-only card branch")
	}
}

func TestRenderDashboardUsesSingleColumnCollapsibleAccountDetails(t *testing.T) {
	page := string(RenderDashboard(300))
	for _, want := range []string{
		`.result-grid{display:grid;grid-template-columns:minmax(0,1fr)`,
		`repeat(auto-fit,minmax(220px,1fr))`,
		`repeat(auto-fit,minmax(200px,1fr))`,
		`account-detail-collapse`,
		`grid-template-rows:0fr`,
		`grid-template-rows:1fr`,
		`var detailKeys = !result.error ? extraDetailKeys(result) : [];`,
		`detailButton.setAttribute("aria-expanded", "false")`,
		`detailButton.setAttribute("aria-controls", detailsID)`,
		`collapse.setAttribute("aria-hidden", "true")`,
		`collapse.setAttribute("inert", "")`,
		`查看账户明细`,
		`收起账户明细`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("dashboard is missing single-column disclosure marker %q", want)
		}
	}
	if strings.Contains(page, `.result-card:hover{border-color:var(--border-hover);box-shadow:var(--shadow-lg);transform:`) {
		t.Fatal("full-width result cards should not jump vertically on hover")
	}
}

func TestRenderDashboardMarksConsoleOnlyProviders(t *testing.T) {
	page := string(RenderDashboard(300))
	for _, want := range []string{
		`"status":"console_only"`,
		`仅控制台可查`,
		`官方未提供模型 API Key 余额查询接口`,
		`当前模型密钥不能直接查询余额`,
		`query-help`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("dashboard is missing console-only provider marker %q", want)
		}
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
	for _, want := range []string{
		`textContent`,
		`redactSecrets`,
		`maskKey`,
		`row.appendChild(element("dd", "", redactSecrets(result.extra[key])))`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("dashboard is missing credential-safe rendering marker %q", want)
		}
	}
}
