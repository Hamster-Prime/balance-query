package ui

import (
	"regexp"
	"strings"
	"testing"
)

func TestRenderDashboardUsesAIProviderManagementAPIs(t *testing.T) {
	page := string(RenderDashboard(300))
	for _, want := range []string{
		`/openai-compatibility`,
		`/claude-api-key`,
		`/xai-api-key`,
		`/codex-api-key`,
		`/gemini-api-key`,
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
		`AI 提供商`,
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

func TestRenderDashboardDoesNotShowQuotaWindowCounts(t *testing.T) {
	page := string(RenderDashboard(300))
	for _, unwanted := range []string{
		`group.windows.length + " 个周期"`,
		`windowCount + " 个配额周期"`,
		`detailCount + " 项账户详情"`,
		`个提供商尚未选择余额查询类型`,
		`id="overview-notice"`,
	} {
		if strings.Contains(page, unwanted) {
			t.Fatalf("dashboard still renders unwanted count or notice marker %q", unwanted)
		}
	}
	for _, want := range []string{
		`var countSummary =`,
		`if (!countSummary) return display;`,
		`if (balanceText) overview.appendChild`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("dashboard is missing count-summary suppression marker %q", want)
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

	failedBranch := regexp.MustCompile(`(?s)if\s*\(failed\).*?\}\s*else\s*\{.*?renderQuotaGroups`)
	if !failedBranch.MatchString(page) {
		t.Fatal("dashboard should keep quota groups out of the failed card branch")
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

func TestRenderDashboardGroupsOverviewByCategoryAndMultipleKeys(t *testing.T) {
	page := string(RenderDashboard(300))
	for _, want := range []string{
		`id="provider-jump-nav"`,
		`aria-label="AI 提供商分组"`,
		`overviewResultGroups`,
		`result.provider_key`,
		`provider.mappingKey`,
		`var serviceKey = category + "\u0000" + baseURL + "\u0000" + queryType;`,
		`if (service.names.length > 1) service.provider.name = serviceName`,
		`overview-category-list`,
		`category-toggle`,
		`var expanded = owns(state.expandedCategories, group.category) ? Boolean(state.expandedCategories[group.category]) : true;`,
		`if (service.results.length === 1) content.appendChild(resultCard`,
		`renderProviderBundle(service.provider, service.results`,
		`var expanded = owns(state.expandedProviders, provider.mappingKey) ? Boolean(state.expandedProviders[provider.mappingKey]) : false;`,
		`bundle-summary`,
		`bundle-collapse`,
		`展开密钥`,
		`scrollIntoView`,
		`prefers-reduced-motion: reduce`,
		`aria-current`,
		`IntersectionObserver`,
		`section.setAttribute("aria-labelledby", toggleID)`,
		`section.setAttribute("aria-labelledby", bundleTitleID)`,
		`toggle.focus({ preventScroll:true })`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("dashboard is missing grouped overview marker %q", want)
		}
	}
}

func TestRenderDashboardUsesConservativeMultipleKeyAggregates(t *testing.T) {
	page := string(RenderDashboard(300))
	for _, want := range []string{
		`function bundleSummaryMetrics`,
		`function quotaBucketMetrics`,
		`item.aggregation_scope === "key"`,
		`result.has_balance`,
		`result.has_cost`,
		`coverage + "/" + expectedCount`,
		`entry.fetchedAt`,
		`quotaResetNode(item, fetchedAt)`,
		`if (!timestamp && item.reset_at)`,
		`按密钥合计`,
		`按密钥总额度`,
		`最低剩余`,
		`平均剩余`,
		`剩余范围`,
		`canonicalQuotaUnit`,
		`result.balance_usd`,
		`已返回合计剩余`,
		`剩余数值覆盖`,
		`百分比覆盖`,
		`hasPercentOnly`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("dashboard is missing multi-key aggregate marker %q", want)
		}
	}
	if strings.Contains(page, `remainingPercents.reduce(function (sum, value) { return sum + value; }, 0)`) {
		t.Fatal("dashboard must not present summed percentages as a quota total")
	}
}

func TestRenderDashboardGroupsProvidersByCategoryAndServiceAddress(t *testing.T) {
	page := string(RenderDashboard(300))
	for _, want := range []string{
		`providerCategoryLabel`,
		`groupedProviders`,
		`"openai-compatibility":"OpenAI 兼容"`,
		`"claude-api-key":"Claude"`,
		`"xai-api-key":"xAI"`,
		`"codex-api-key":"Codex"`,
		`"gemini-api-key":"Gemini"`,
		`"xai-api-key":"https://api.x.ai/v1"`,
		`provider-group-row`,
		`provider-group-title`,
		`按服务地址分别配置`,
		`provider.baseUrl || "官方默认服务地址"`,
		`normalizeBaseForKey(left.baseUrl).localeCompare`,
		`category === "OpenAI 兼容" ? identity`,
		`var normalizedPath = parsed.pathname.replace`,
		`mappedQueryType`,
		`legacyMappingKey`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("dashboard is missing provider grouping marker %q", want)
		}
	}
	if strings.Contains(page, `var groupKey = baseURL.toLowerCase()`) {
		t.Fatal("dashboard must preserve case-sensitive URL paths while grouping services")
	}
}

func TestRenderDashboardSkipsNativeKeysDisabledInCPA(t *testing.T) {
	page := string(RenderDashboard(300))
	for _, want := range []string{
		`keyEntryDisabled`,
		`entry["excluded-models"]`,
		`if (keyEntry.disabled) return;`,
		`entry.disabled ? " disabled"`,
		`该密钥已在 CPA 中停用`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("dashboard is missing disabled-key behavior %q", want)
		}
	}
}

func TestRenderDashboardOmitsConsoleOnlyQueryOptions(t *testing.T) {
	page := string(RenderDashboard(300))
	for _, unavailable := range []string{
		`"value":"minimax_api"`,
		`"value":"xiaomi_api"`,
		`"value":"xiaomi_token"`,
		`"value":"longcat"`,
		`"value":"opencode"`,
		`"value":"volcengine"`,
		`仅控制台可查`,
		`definition.label + (definition.status === "console_only"`,
		`class="query-help"`,
	} {
		if strings.Contains(page, unavailable) {
			t.Fatalf("dashboard still offers console-only query option %q", unavailable)
		}
	}
	if !strings.Contains(page, `"value":"claude_admin"`) {
		t.Fatal("dashboard is missing the official Claude Admin query option")
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
