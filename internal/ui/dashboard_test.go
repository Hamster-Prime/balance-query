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
		`/balance-query/config-apply`,
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

func TestRenderDashboardIncludesManualKeyQueryAboveOverview(t *testing.T) {
	page := string(RenderDashboard(300))
	manualQuery := strings.Index(page, `id="manual-query-form"`)
	summary := strings.Index(page, `class="summary"`)
	if manualQuery < 0 || summary < 0 || manualQuery > summary {
		t.Fatalf("manual query form should appear above the overview summary (manual=%d summary=%d)", manualQuery, summary)
	}
	for _, want := range []string{
		`自主查询`,
		`id="manual-query-type"`,
		`id="manual-api-key" type="password"`,
		`id="manual-base-url-field"`,
		`id="manual-query-result"`,
		`var MANUAL_QUERY_PATH = "/balance-query/manual-query";`,
		`requires_base_url`,
		`function queryManualBalance(event)`,
		`sanitizeSnapshotValue(data, [apiKey])`,
		`target.appendChild(resultCard(result, 0, "manual-account-details"))`,
		`body:JSON.stringify(payload)`,
		`resetManualQuery(true)`,
		`.manual-query-form{display:flex`,
		`.manual-query-fields{min-width:0;display:grid`,
		`.manual-query-submit{width:100%}`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("dashboard is missing manual query marker %q", want)
		}
	}
	for _, queryType := range []string{`"value":"sub2api"`, `"value":"newapi"`} {
		definition := regexp.MustCompile(regexp.QuoteMeta(queryType) + `[^}]*"requires_base_url":true`)
		if !definition.MatchString(page) {
			t.Fatalf("manual query definition %q should require a service URL", queryType)
		}
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
		`if (!countSummary && (!hasWindows || isWalletSummary)) return display;`,
		`if (balanceText) {`,
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
		`Object.keys(result.extra).filter`,
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
		`no failed detail toggle`:  `var\s+detailKeys\s*=\s*!failed\s*\?\s*extraDetailKeys\(result\)\s*:\s*\[\]`,
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
		`var detailKeys = !failed ? extraDetailKeys(result) : [];`,
		`detailButton.setAttribute("aria-expanded", "false")`,
		`detailButton.setAttribute("aria-controls", detailsID)`,
		`collapse.setAttribute("aria-hidden", "true")`,
		`collapse.setAttribute("inert", "")`,
		`function setDisclosureState(button, collapse, expanded)`,
		`var wasExpanded = collapse.getAttribute("aria-hidden") === "false"`,
		`collapse.removeAttribute("inert")`,
		`collapse.addEventListener("transitionend", finish)`,
		`if (collapse.getAttribute("aria-hidden") === "true") collapse.setAttribute("inert", "")`,
		`setDisclosureState(detailButton, collapse, nextExpanded)`,
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
	accountExpanded := regexp.MustCompile(`\.account-detail-collapse\[aria-hidden="false"\]\{[^}]*grid-template-rows:1fr`)
	if !accountExpanded.MatchString(page) {
		t.Fatal("account detail disclosure must animate between compatible 0fr and 1fr tracks")
	}
}

func TestRenderDashboardNamesSingleAndMultipleEnabledKeys(t *testing.T) {
	page := string(RenderDashboard(300))
	for _, want := range []string{
		`var enabledKeys = provider.keys.map(function (keyEntry, originalIndex)`,
		`return { entry:keyEntry, originalIndex:originalIndex };`,
		`}).filter(function (item) { return !item.entry.disabled; });`,
		`enabledKeys.forEach(function (item, displayIndex)`,
		`var accountName = enabledKeys.length === 1 ? provider.name : provider.name + " · 密钥 " + (displayIndex + 1);`,
		`(item.originalIndex + 1)`,
		`account_name: accountName`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("dashboard is missing enabled-key naming marker %q", want)
		}
	}
	if strings.Contains(page, `account_name: provider.name + " · 密钥 " + (index + 1)`) {
		t.Fatal("dashboard still appends a key number to every single-key provider")
	}
}

func TestRenderDashboardGroupsOverviewByCategoryAndMultipleKeys(t *testing.T) {
	page := string(RenderDashboard(300))
	for _, want := range []string{
		`id="provider-jump-nav"`,
		`aria-label="AI 提供商分组"`,
		`activeOverviewCategory`,
		`nav.setAttribute("role", "tablist")`,
		`tab.setAttribute("role", "tab")`,
		`tab.setAttribute("aria-selected", String(selected))`,
		`section.setAttribute("role", "tabpanel")`,
		`section.hidden = group.category !== state.activeOverviewCategory`,
		`activateOverviewCategory(group.category, false)`,
		`event.key === "ArrowRight"`,
		`event.key === "Home"`,
		`overviewResultGroups`,
		`result.provider_key`,
		`provider.mappingKey`,
		`var serviceKey = category + "\u0000" + baseURL + "\u0000" + queryType;`,
		`if (service.names.length > 1) service.provider.name = serviceName`,
		`overview-category-list`,
		`if (service.results.length === 1) content.appendChild(resultCard`,
		`renderProviderBundle(service.provider, service.results`,
		`var expanded = owns(state.expandedProviders, provider.mappingKey) ? Boolean(state.expandedProviders[provider.mappingKey]) : false;`,
		`bundle-summary`,
		`bundle-collapse`,
		`展开密钥`,
		`section.setAttribute("aria-labelledby", tabID)`,
		`section.setAttribute("aria-labelledby", bundleTitleID)`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("dashboard is missing grouped overview marker %q", want)
		}
	}
	for _, unwanted := range []string{
		`IntersectionObserver`,
		`expandedCategories`,
		`category-toggle`,
		`aria-current`,
	} {
		if strings.Contains(page, unwanted) {
			t.Fatalf("dashboard still uses obsolete all-category navigation marker %q", unwanted)
		}
	}
}

func TestRenderDashboardSuppressesDuplicatedQuotaSummaries(t *testing.T) {
	page := string(RenderDashboard(300))
	for _, want := range []string{
		`function typedWalletBalance(result)`,
		`var genericBalancePresent = Boolean(result.has_balance_amount) || owns(result, "balance_amount")`,
		`return { amount:genericAmount, unit:result.balance_currency || "credits" };`,
		`if (typedWallet) return amountWithUnit(typedWallet.amount, typedWallet.unit);`,
		`var balancePresent = Boolean(result.has_balance) || owns(result, "balance_usd")`,
		`return "钱包余额 " + amountWithUnit(balanceAmount, "usd")`,
		`var hasWindows = Array.isArray(result.quota_windows) && result.quota_windows.length > 0`,
		`var isWalletSummary = /^钱包余额`,
		`if (Array.isArray(result.quota_windows) && result.quota_windows.length > 0) return ""`,
		`if (plan === "钱包余额") return ""`,
		`if (hasWindows && (plan === "账户额度" || plan === "密钥独立额度")) return ""`,
		`function renderWalletBalance(card, result)`,
		`var box = element("div", "quota-window wallet-window")`,
		`box.setAttribute("aria-label", "钱包余额")`,
		`return [typedWalletBalance(result) ? "钱包余额" : "", titlePlanText(result), titleDateText(result)].filter(Boolean);`,
		`var renderedWallet = renderWalletBalance(card, result);`,
		`var balanceText = renderedWallet ? "" : formatBalance(result);`,
		`if (balanceText) {`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("dashboard is missing summary de-duplication marker %q", want)
		}
	}
	for _, unwanted := range []string{
		`return "钱包余额 " + amountWithUnit(genericAmount, result.balance_currency || "credits")`,
		`renderAccountMeta(overview, result)`,
		`label:"查询状态"`,
	} {
		if strings.Contains(page, unwanted) {
			t.Fatalf("dashboard still renders redundant overview content %q", unwanted)
		}
	}
}

func TestRenderDashboardPlacesPlanAndResetInTitle(t *testing.T) {
	page := string(RenderDashboard(300))
	for _, want := range []string{
		`result-title-row`,
		`bundle-title-row`,
		`title-meta-chip`,
		`appendTitleMetadata(titleRow, resultTitleMetadata(result))`,
		`appendTitleMetadata(titleRow, commonTitleMetadata(results))`,
		`var successful = results.filter(function (result) { return !result.error; });`,
		`var walletValues = successful.map(function (result) { return typedWalletBalance(result) ? "钱包余额" : ""; });`,
		`if (walletValues.length && walletValues.every(Boolean)) values.push("钱包余额");`,
		`owns(extra, "密钥到期") || owns(extra, "套餐到期") ? "到期 " : "重置 "`,
		`key === "套餐名称" && result.plan`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("dashboard is missing title metadata marker %q", want)
		}
	}
	if strings.Contains(page, `class="account-meta"`) || strings.Contains(page, `function renderAccountMeta`) {
		t.Fatal("dashboard still reserves a separate row for account plan/reset metadata")
	}
	if strings.Contains(page, `if (successful.length !== results.length) return [];`) {
		t.Fatal("failed keys should not hide common metadata from successful wallet results")
	}
}

func TestRenderDashboardFramesMultipleKeyBundles(t *testing.T) {
	page := string(RenderDashboard(300))
	for name, pattern := range map[string]string{
		"complete bundle frame":     `(?s)\.provider-bundle\{[^}]*padding:15px[^}]*border:1px solid[^}]*border-radius:12px[^}]*background:`,
		"separated nested key rows": `(?s)\.bundle-result-grid\{[^}]*gap:10px[^}]*padding-top:14px.*?\.provider-bundle \.result-card\{[^}]*border:1px solid[^}]*border-left:3px solid[^}]*border-radius:9px[^}]*background:`,
		"visible failed key row":    `(?s)\.provider-bundle \.result-card\.error\{[^}]*border-color:var\(--failure-badge-border\)[^}]*border-left-color:var\(--error-color\)`,
	} {
		if !regexp.MustCompile(pattern).MatchString(page) {
			t.Fatalf("dashboard is missing %s (%s)", name, pattern)
		}
	}
}

func TestRenderDashboardUsesRemainingQuotaForProgress(t *testing.T) {
	page := string(RenderDashboard(300))
	for _, want := range []string{
		`function quotaRemainingPercent(item)`,
		`function quotaWindowIsExhausted(item)`,
		`function quotaWindowState(item)`,
		`function quotaResultState(items)`,
		`if (/已用尽|exhausted/i.test(String(item.status || ""))) return true`,
		`"partial-exhausted"`,
		`"全部配额已用尽"`,
		`"部分配额已用尽"`,
		`"当前套餐未提供此项"`,
		`function remainingProgressClass(percent)`,
		`if (percent <= 15) return " critical"`,
		`if (percent <= 50) return " warning"`,
		`track.setAttribute("aria-label", translateWindowLabel(item.label) + "剩余额度")`,
		`track.setAttribute("aria-valuenow", String(Math.round(visualRemaining)))`,
		`track.setAttribute("aria-valuetext", formatAmount(progressRemaining, "%") + (item.aggregate_window ? " 综合剩余" : " 剩余"))`,
		`bar.style.width = visualRemaining.toFixed(1) + "%"`,
		`var percent = clampPercent((total - used) / total * 100)`,
		`track.setAttribute("aria-label", "令牌剩余额度")`,
		`.progress-bar.warning`,
		`.progress-bar.critical`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("dashboard is missing remaining-quota progress marker %q", want)
		}
	}
	for _, unwanted := range []string{
		`bar.style.width = usedPercent.toFixed(1)`,
		`aria-label", translateWindowLabel(item.label) + "使用进度`,
		`.progress-bar.medium`,
		`.progress-bar.high`,
	} {
		if strings.Contains(page, unwanted) {
			t.Fatalf("dashboard still uses consumed-quota progress marker %q", unwanted)
		}
	}
}

func TestRenderDashboardSumsSuccessfulMultipleKeyQuotaWindows(t *testing.T) {
	page := string(RenderDashboard(300))
	for _, want := range []string{
		`function aggregateQuotaWindows(results)`,
		`var successful = results.filter(function (result) { return !result.error; });`,
		`function quotaAggregationScope(item)`,
		`function quotaAggregationDimension(item)`,
		`var stableID = String(item.aggregation_key || "").trim()`,
		`var baseKey = stableID + "\u0000" + scope + "\u0000" + dimension;`,
		`var representedCount = uniqueResultCount(bucket.entries);`,
		`var missingCount = Math.max(0, successful.length - representedCount);`,
		`var unlimitedEntries = bucket.entries.filter`,
		`var unknownEntries = bucket.entries.filter`,
		`aggregate_missing_count:missingCount`,
		`aggregate_failed_count:failedCount`,
		`if (unlimitedEntries.length) {`,
		`aggregate.unlimited = true;`,
		`if (bucket.scope !== "key") {`,
		`aggregate.aggregate_range = true;`,
		`var exactEntries = finiteEntries.filter`,
		`if (exactEntries.length === finiteEntries.length) {`,
		`aggregate.progress_remaining_percent = aggregate.total > 0 ? aggregate.remaining / aggregate.total * 100 : 0;`,
		`aggregate.remaining_percent = percentSum;`,
		`aggregate.capacity_percent = capacitySum;`,
		`percentSum / capacitySum * 100`,
		`function quotaResetSummary(entries)`,
		`aggregate.reset_staggered = true`,
		`function quotaCoverageText(item)`,
		`个未返回此项`,
		`var progressRemaining = quotaProgressRemainingPercent(item);`,
		`var aggregateWindows = aggregateQuotaWindows(results);`,
		`bundleSummaryMetrics(results, aggregateWindows.length > 0)`,
		`renderQuotaGroups(section, { quota_windows:aggregateWindows`,
		`function bundleSummaryMetrics`,
		`result.has_balance`,
		`result.has_cost`,
		`quotaResetNode(item, fetchedAt)`,
		`var duplicateReset = Boolean(item.reset_at && accountResetAt`,
		`var resetNode = duplicateReset ? null : quotaResetNode(item, fetchedAt)`,
		`Boolean(result.has_balance_amount) || owns(result, "balance_amount")`,
		`canonicalQuotaUnit(genericPresent ? result.balance_currency : "usd")`,
		`if (!successful.length) return []`,
		`planValues.every(Boolean)`,
		`summaryValues.every(Boolean)`,
		`canonicalQuotaUnit`,
		`result.balance_usd`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("dashboard is missing multi-key aggregate marker %q", want)
		}
	}

	unlimitedBeforeFiniteTotals := regexp.MustCompile(`(?s)var unlimitedEntries = bucket\.entries\.filter.*?if \(unlimitedEntries\.length\) \{.*?return aggregate;.*?if \(!finiteEntries\.length\)`)
	if !unlimitedBeforeFiniteTotals.MatchString(page) {
		t.Fatal("dashboard must propagate unlimited quota before aggregating finite quota values")
	}
	for _, unwanted := range []string{`quotaBucketMetrics(`, `最低剩余`, `平均剩余`} {
		if strings.Contains(page, unwanted) {
			t.Fatalf("dashboard still contains obsolete minimum/range quota aggregation %q", unwanted)
		}
	}
}

func TestRenderDashboardKeepsReconnectLabelAndReusesRefreshIcon(t *testing.T) {
	page := string(RenderDashboard(300))
	for _, want := range []string{
		`id="reconnect-button"`,
		`<span class="btn-label">重新连接</span>`,
		`.btn.is-refreshing .refresh-icon{animation:spin .9s linear infinite}`,
		`var refreshIcon = button.querySelector(".refresh-icon");`,
		`button.classList.toggle("is-refreshing", Boolean(busy && refreshIcon));`,
		`if (refreshIcon) {`,
		`if (oldSpinner) oldSpinner.remove();`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("dashboard is missing responsive toolbar marker %q", want)
		}
	}
	for _, unwanted := range []string{
		`<span class="btn-label optional">重新连接</span>`,
		`.btn-label.optional{display:none}`,
	} {
		if strings.Contains(page, unwanted) {
			t.Fatalf("dashboard still hides or duplicates the reconnect/refresh control %q", unwanted)
		}
	}

	refreshBranch := regexp.MustCompile(`(?s)var refreshIcon = button\.querySelector\("\.refresh-icon"\);.*?if \(refreshIcon\) \{.*?return;.*?if \(busy && !oldSpinner\)`)
	if !refreshBranch.MatchString(page) {
		t.Fatal("refresh busy state must return before the generic spinner is inserted")
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
		`个服务地址 · 分别配置`,
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

func TestRenderDashboardSettingsUseCategoryTabsAndMobileCards(t *testing.T) {
	page := string(RenderDashboard(300))
	for _, want := range []string{
		`id="settings-provider-nav"`,
		`aria-label="查询设置提供商分组"`,
		`id="settings-sheet" class="settings-sheet"`,
		`settings-policy-card`,
		`activeSettingsCategory`,
		`function activateSettingsCategory(category, focusTab)`,
		`function renderSettingsNavigation(groups)`,
		`function revealSettingsTab(tab, nav)`,
		`tab.setAttribute("aria-controls", "settings-sheet")`,
		`sheet.setAttribute("aria-labelledby", settingsTabID(group.category))`,
		`sheet.setAttribute("role", "tabpanel")`,
		`sheet.removeAttribute("role")`,
		`provider-settings-row`,
		`settings-field-label`,
		`.settings-table .provider-settings-row td+td`,
		`.settings-provider-nav{flex-wrap:nowrap;overflow-x:auto`,
		`scrollbar-width:thin`,
		`.settings-sheet{border:0;background:transparent;box-shadow:none;overflow:visible}`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("dashboard is missing settings category/mobile marker %q", want)
		}
	}
	if strings.Contains(page, `table,thead,tbody,tr,th,td{display:block}`) {
		t.Fatal("mobile table rules still leak outside the settings table")
	}
}

func TestRenderDashboardDirectlyAppliesRuntimeConfigAndPreservesInFlightDrafts(t *testing.T) {
	page := string(RenderDashboard(300))
	for _, want := range []string{
		`var RUNTIME_CONFIG_APPLY_PATH = "/balance-query/config-apply";`,
		`function applyRuntimeConfig(config, controller, credentials)`,
		`method: "POST"`,
		`body: JSON.stringify(normalizeConfig(config))`,
		`function withConfigSaveLock(credentials, controller, callback)`,
		`var lockName = "balance-query-config::v2::"`,
		`ifAvailable:true`,
		`return withConfigSaveLock(credentials, controller, function (assertOwnership)`,
		`var CONFIG_SAVE_STEP_TIMEOUT = 4000;`,
		`var CONFIG_APPLY_TIMEOUT = 3000;`,
		`var CONFIG_SAVE_TOTAL_TIMEOUT = 10000;`,
		`var leaseMilliseconds = 12000;`,
		`assertOwnership();`,
		`setText(byID("save-state"), "已写入，正在应用")`,
		`return applyAndReconcileRuntimeConfig(nextConfig, controller, credentials, assertOwnership);`,
		`state.runtimeApplyFailed = true;`,
		`var submittedRevision = state.draftRevision;`,
		`if (state.draftRevision === submittedRevision)`,
		`rebaseDraftMappings(state.config.provider_mappings`,
		`mergeSubmittedConfig(normalizeConfig(latestRaw)`,
		`state.draftRevision += 1;`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("dashboard is missing verified-save marker %q", want)
		}
	}
	for _, unwanted := range []string{
		`function waitForRuntimeConfig(`,
		`function waitForSynchronizedConfig(`,
		`插件配置同步中`,
	} {
		if strings.Contains(page, unwanted) {
			t.Fatalf("dashboard still contains obsolete runtime polling marker %q", unwanted)
		}
	}
}

func TestRenderDashboardSkipsNativeKeysDisabledInCPA(t *testing.T) {
	page := string(RenderDashboard(300))
	for _, want := range []string{
		`keyEntryDisabled`,
		`entry["excluded-models"]`,
		`}).filter(function (item) { return !item.entry.disabled; });`,
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

func TestRenderDashboardRestoresSafeSessionSnapshotsAndCoordinatesRequests(t *testing.T) {
	page := string(RenderDashboard(300))
	for _, want := range []string{
		`var QUERY_BATCH_SIZE = 128;`,
		`var QUERY_BATCH_MAX_BYTES = 900 * 1024;`,
		`var RESULT_SNAPSHOT_KEY = "balance-query-results::v1";`,
		`function fallbackSHA256(value)`,
		`function sha256Hex(value)`,
		`function sameOriginSessionStorage()`,
		`function persistResultSnapshot(accounts, results, ttlSeconds, credentials)`,
		`function readResultSnapshot(accounts, ttlSeconds, credentials)`,
		`(!Array.isArray(result.warnings) || result.warnings.length === 0)`,
		`var RESULT_SNAPSHOT_VERSION = 2;`,
		`function readResultSnapshotPreview(credentials)`,
		`if (snapshot.fresh) return;`,
		`if (generation !== state.loadGeneration`,
		`if (generation !== state.queryGeneration`,
		`saveGeneration: 0`,
		`function cancelSaveRequests()`,
		`var loadGeneration = state.loadGeneration;`,
		`var generation = ++state.saveGeneration;`,
		`signal: controller.signal`,
		`generation !== state.saveGeneration || loadGeneration !== state.loadGeneration`,
		`cancelSaveRequests();`,
		`function accountBatches(accounts)`,
		`function queryTimeoutForBatch(accountCount)`,
		`responseErrorMessage(data, response.status, requestCredentials)`,
		`optionalApiFetch("/proxy-url"`,
		`error.proxyConfigUnavailable = true`,
		`draftTTL: String(INITIAL_TTL)`,
		`var nextMappings = Object.assign({}, latestConfig.provider_mappings);`,
		`if (!providerMappingChanged(provider, baselineConfig.provider_mappings, submittedMappings)) return;`,
		`if (mappingChanged) cancelQueryRequests();`,
		`mapping_key: resolvedMapping.key`,
		`var keyCount = displayAccounts().length;`,
		`function latestFetchedAt(results)`,
		`failure.title`,
		`failure.reason`,
		`failure.suggestion`,
		`failure.retry_after_seconds`,
		`建议 " + Math.ceil(retryAfter) + " 秒后重试`,
		`failureKindLabel(failure.kind)`,
		`function resultWarnings(result)`,
		`var warningItems = failed ? [] : resultWarnings(result);`,
		`var detailKeys = !failed ? extraDetailKeys(result) : [];`,
		`partial ? "部分数据"`,
		`renderResultWarnings(card, warningItems);`,
		`"部分数据未获取（" + warnings.length + " 项）"`,
		`"查询成功 · " + partialCount + " 个部分数据"`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("dashboard is missing snapshot/request-state marker %q", want)
		}
	}
	for _, unwanted := range []string{
		`localStorage.setItem(RESULT_SNAPSHOT_KEY`,
		`state.results = [];
      renderResults();
      toast(error`,
	} {
		if strings.Contains(page, unwanted) {
			t.Fatalf("dashboard contains unsafe or destructive refresh behavior %q", unwanted)
		}
	}
}
