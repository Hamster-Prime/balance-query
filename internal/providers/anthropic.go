package providers

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/Hamster-Prime/balance-query/internal/balance"
)

const anthropicVersion = "2023-06-01"

const claudeReportTimeout = 75 * time.Second

// ClaudeAdmin queries Anthropic's organization Usage & Cost Admin API. These
// endpoints report historical organization activity; they do not expose a
// wallet balance or subscription allowance.
type ClaudeAdmin struct {
	BaseURL string
}

type claudeUsageResponse struct {
	Data []struct {
		StartingAt string `json:"starting_at"`
		EndingAt   string `json:"ending_at"`
		Results    []struct {
			UncachedInputTokens  int64 `json:"uncached_input_tokens"`
			CacheReadInputTokens int64 `json:"cache_read_input_tokens"`
			CacheCreation        struct {
				Ephemeral1HInputTokens int64 `json:"ephemeral_1h_input_tokens"`
				Ephemeral5MInputTokens int64 `json:"ephemeral_5m_input_tokens"`
			} `json:"cache_creation"`
			OutputTokens  int64  `json:"output_tokens"`
			Model         string `json:"model"`
			ServerToolUse struct {
				WebSearchRequests int64 `json:"web_search_requests"`
			} `json:"server_tool_use"`
		} `json:"results"`
	} `json:"data"`
	HasMore  bool   `json:"has_more"`
	NextPage string `json:"next_page"`
}

type claudeCostResponse struct {
	Data []struct {
		StartingAt string             `json:"starting_at"`
		EndingAt   string             `json:"ending_at"`
		Results    []claudeCostResult `json:"results"`
	} `json:"data"`
	HasMore  bool   `json:"has_more"`
	NextPage string `json:"next_page"`
}

type claudeCostResult struct {
	Amount      string `json:"amount"`
	Currency    string `json:"currency"`
	CostType    string `json:"cost_type"`
	Description string `json:"description"`
	Model       string `json:"model"`
	TokenType   string `json:"token_type"`
}

type claudeUsageTotals struct {
	UncachedInput int64
	CacheRead     int64
	Cache1H       int64
	Cache5M       int64
	Output        int64
	WebSearches   int64
	TotalTokens   int64
	ByModel       map[string]*claudeModelUsage
}

type claudeModelUsage struct {
	Input       int64
	Output      int64
	WebSearches int64
}

type claudeUsageOutcome struct {
	totals claudeUsageTotals
	err    error
}

type claudeCostOutcome struct {
	totalUSD float64
	details  map[string]float64
	err      error
}

func (totals claudeUsageTotals) inputTokens() int64 {
	return totals.TotalTokens - totals.Output
}

func (totals claudeUsageTotals) totalTokens() int64 {
	return totals.TotalTokens
}

func (c ClaudeAdmin) Fetch(authID, token, proxyURL string) balance.Result {
	label := balance.ProviderLabel[balance.ProviderClaudeAdmin]
	if strings.TrimSpace(token) == "" {
		return errResult(authID, label, newProviderError(balance.FailureAuthentication, "管理员密钥为空", 0, "", nil))
	}

	baseURL := strings.TrimSpace(c.BaseURL)
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	usageEndpoint, err := serviceEndpoint(baseURL, "/v1/organizations/usage_report/messages")
	if err != nil {
		return errResult(authID, label, err)
	}
	costEndpoint, err := serviceEndpoint(baseURL, "/v1/organizations/cost_report")
	if err != nil {
		return errResult(authID, label, err)
	}

	endingAt := time.Now().UTC().Truncate(time.Hour)
	startingAt := endingAt.Add(-30 * 24 * time.Hour)
	commonQuery := url.Values{
		"starting_at":  []string{startingAt.Format(time.RFC3339)},
		"ending_at":    []string{endingAt.Format(time.RFC3339)},
		"bucket_width": []string{"1d"},
		"limit":        []string{"31"},
	}
	usageQuery := cloneURLValues(commonQuery)
	usageQuery.Add("group_by[]", "model")
	costQuery := cloneURLValues(commonQuery)
	costQuery.Add("group_by[]", "description")
	headers := map[string]string{
		"x-api-key":         token,
		"anthropic-version": anthropicVersion,
	}

	usageResult := make(chan claudeUsageOutcome, 1)
	costResult := make(chan claudeCostOutcome, 1)
	go func() {
		totals, fetchErr := fetchClaudeUsage(usageEndpoint, usageQuery, proxyURL, headers)
		usageResult <- claudeUsageOutcome{totals: totals, err: fetchErr}
	}()
	go func() {
		totalUSD, details, fetchErr := fetchClaudeCosts(costEndpoint, costQuery, proxyURL, headers)
		costResult <- claudeCostOutcome{totalUSD: totalUSD, details: details, err: fetchErr}
	}()

	usage := <-usageResult
	cost := <-costResult
	usageTotals, usageErr := usage.totals, usage.err
	costUSD, costDetails, costErr := cost.totalUSD, cost.details, cost.err
	if usageErr != nil && costErr != nil {
		return errResult(authID, label, combinedClaudeReportError(usageErr, costErr))
	}

	result := balance.Result{
		Provider:  label,
		AuthID:    authID,
		Plan:      "组织管理员 API",
		Extra:     map[string]string{},
		FetchedAt: time.Now(),
	}
	if costErr == nil {
		result.CostUSD = costUSD
		result.HasCost = true
		result.CostScope = "organization"
		result.QuotaDisplay = fmt.Sprintf("近 30 天费用 %.2f USD", costUSD)
		result.Extra["近 30 天费用"] = fmt.Sprintf("%.2f USD", costUSD)
		for description, amount := range costDetails {
			result.Extra["费用 "+description] = fmt.Sprintf("%.2f USD", amount)
		}
	} else {
		warning := providerWarning("费用查询", costErr)
		result.Warnings = append(result.Warnings, warning)
		result.Extra["费用查询"] = "未成功，详见告警：" + warning.Title
	}
	if usageErr == nil {
		if result.QuotaDisplay == "" {
			result.QuotaDisplay = fmt.Sprintf("近 30 天使用 %d 令牌", usageTotals.totalTokens())
		}
		appendClaudeUsageDetails(&result, usageTotals)
	} else {
		warning := providerWarning("用量查询", usageErr)
		result.Warnings = append(result.Warnings, warning)
		result.Extra["用量查询"] = "未成功，详见告警：" + warning.Title
	}
	return result
}

func combinedClaudeReportError(usageErr, costErr error) error {
	message := fmt.Sprintf("用量查询失败：%v；费用查询失败：%v", usageErr, costErr)
	usageFailure := providerWarning("", usageErr)
	costFailure := providerWarning("", costErr)
	kind := balance.FailureUnknown
	status := 0
	code := ""
	retryAfter := max(usageFailure.RetryAfterSeconds, costFailure.RetryAfterSeconds)
	requestID := ""
	if usageFailure.Kind == costFailure.Kind {
		kind = usageFailure.Kind
		if usageFailure.HTTPStatus == costFailure.HTTPStatus {
			status = usageFailure.HTTPStatus
		}
		if usageFailure.ProviderCode == costFailure.ProviderCode {
			code = usageFailure.ProviderCode
		}
		if usageFailure.RequestID == costFailure.RequestID {
			requestID = usageFailure.RequestID
		}
	}
	return &ProviderError{
		Kind:              kind,
		Message:           safeProviderMessage(message),
		HTTPStatus:        status,
		ProviderCode:      code,
		RequestID:         requestID,
		RetryAfterSeconds: retryAfter,
		Cause:             errors.Join(usageErr, costErr),
	}
}

func fetchClaudeUsage(endpoint string, query url.Values, proxyURL string, headers map[string]string) (claudeUsageTotals, error) {
	ctx, cancel := context.WithTimeout(context.Background(), claudeReportTimeout)
	defer cancel()
	return fetchClaudeUsageWithContext(ctx, endpoint, query, proxyURL, headers)
}

func fetchClaudeUsageWithContext(ctx context.Context, endpoint string, query url.Values, proxyURL string, headers map[string]string) (claudeUsageTotals, error) {
	totals := claudeUsageTotals{ByModel: map[string]*claudeModelUsage{}}
	page := ""
	for requestCount := 0; requestCount < 20; requestCount++ {
		pageQuery := cloneURLValues(query)
		if page != "" {
			pageQuery.Set("page", page)
		}
		var response claudeUsageResponse
		if err := getJSONWithHeadersContext(ctx, endpoint+"?"+pageQuery.Encode(), proxyURL, headers, &response); err != nil {
			return claudeUsageTotals{}, err
		}
		for _, bucket := range response.Data {
			for _, item := range bucket.Results {
				if !addNonnegativeInt64(&totals.TotalTokens, item.UncachedInputTokens, item.CacheReadInputTokens,
					item.CacheCreation.Ephemeral1HInputTokens, item.CacheCreation.Ephemeral5MInputTokens, item.OutputTokens) {
					return claudeUsageTotals{}, invalidResponseError("用量接口返回了无效或溢出的令牌总数")
				}
				if !addNonnegativeInt64(&totals.UncachedInput, item.UncachedInputTokens) ||
					!addNonnegativeInt64(&totals.CacheRead, item.CacheReadInputTokens) ||
					!addNonnegativeInt64(&totals.Cache1H, item.CacheCreation.Ephemeral1HInputTokens) ||
					!addNonnegativeInt64(&totals.Cache5M, item.CacheCreation.Ephemeral5MInputTokens) ||
					!addNonnegativeInt64(&totals.Output, item.OutputTokens) ||
					!addNonnegativeInt64(&totals.WebSearches, item.ServerToolUse.WebSearchRequests) {
					return claudeUsageTotals{}, invalidResponseError("用量接口返回了无效或溢出的计数")
				}
				model := firstNonEmpty(strings.TrimSpace(item.Model), "未知模型")
				modelTotals := totals.ByModel[model]
				if modelTotals == nil {
					modelTotals = &claudeModelUsage{}
					totals.ByModel[model] = modelTotals
				}
				if !addNonnegativeInt64(&modelTotals.Input, item.UncachedInputTokens, item.CacheReadInputTokens,
					item.CacheCreation.Ephemeral1HInputTokens, item.CacheCreation.Ephemeral5MInputTokens) ||
					!addNonnegativeInt64(&modelTotals.Output, item.OutputTokens) ||
					!addNonnegativeInt64(&modelTotals.WebSearches, item.ServerToolUse.WebSearchRequests) {
					return claudeUsageTotals{}, invalidResponseError("用量接口返回了无效或溢出的模型计数")
				}
			}
		}
		if !response.HasMore || strings.TrimSpace(response.NextPage) == "" {
			return totals, nil
		}
		page = response.NextPage
	}
	return claudeUsageTotals{}, invalidResponseError("用量接口分页超过安全上限")
}

func addNonnegativeInt64(target *int64, values ...int64) bool {
	for _, value := range values {
		if value < 0 || *target > math.MaxInt64-value {
			return false
		}
		*target += value
	}
	return true
}

func fetchClaudeCosts(endpoint string, query url.Values, proxyURL string, headers map[string]string) (float64, map[string]float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), claudeReportTimeout)
	defer cancel()
	return fetchClaudeCostsWithContext(ctx, endpoint, query, proxyURL, headers)
}

func fetchClaudeCostsWithContext(ctx context.Context, endpoint string, query url.Values, proxyURL string, headers map[string]string) (float64, map[string]float64, error) {
	totalUSD := 0.0
	details := map[string]float64{}
	page := ""
	for requestCount := 0; requestCount < 20; requestCount++ {
		pageQuery := cloneURLValues(query)
		if page != "" {
			pageQuery.Set("page", page)
		}
		var response claudeCostResponse
		if err := getJSONWithHeadersContext(ctx, endpoint+"?"+pageQuery.Encode(), proxyURL, headers, &response); err != nil {
			return 0, nil, err
		}
		for _, bucket := range response.Data {
			for _, item := range bucket.Results {
				if item.Currency != "" && !strings.EqualFold(item.Currency, "USD") {
					continue
				}
				amountInCents, ok := numberValue(item.Amount)
				if !ok {
					return 0, nil, invalidResponseError("费用金额格式无效")
				}
				amountUSD := amountInCents / 100
				nextTotal := totalUSD + amountUSD
				description := claudeCostDetailLabel(item)
				nextDetail := details[description] + amountUSD
				if _, ok := numberValue(nextTotal); !ok {
					return 0, nil, invalidResponseError("费用合计超出可表示范围")
				}
				if _, ok := numberValue(nextDetail); !ok {
					return 0, nil, invalidResponseError("费用明细合计超出可表示范围")
				}
				totalUSD = nextTotal
				details[description] = nextDetail
			}
		}
		if !response.HasMore || strings.TrimSpace(response.NextPage) == "" {
			return totalUSD, details, nil
		}
		page = response.NextPage
	}
	return 0, nil, invalidResponseError("费用接口分页超过安全上限")
}

func appendClaudeUsageDetails(result *balance.Result, totals claudeUsageTotals) {
	result.Extra["近 30 天总令牌"] = fmt.Sprintf("%d", totals.totalTokens())
	result.Extra["未缓存输入令牌"] = fmt.Sprintf("%d", totals.UncachedInput)
	result.Extra["缓存读取令牌"] = fmt.Sprintf("%d", totals.CacheRead)
	result.Extra["1 小时缓存写入令牌"] = fmt.Sprintf("%d", totals.Cache1H)
	result.Extra["5 分钟缓存写入令牌"] = fmt.Sprintf("%d", totals.Cache5M)
	result.Extra["输出令牌"] = fmt.Sprintf("%d", totals.Output)
	result.Extra["Web 搜索请求"] = fmt.Sprintf("%d 次", totals.WebSearches)

	models := make([]string, 0, len(totals.ByModel))
	for model := range totals.ByModel {
		models = append(models, model)
	}
	sort.Strings(models)
	for _, model := range models {
		usage := totals.ByModel[model]
		value := fmt.Sprintf("输入 %d 令牌 · 输出 %d 令牌", usage.Input, usage.Output)
		if usage.WebSearches > 0 {
			value += fmt.Sprintf(" · Web 搜索 %d 次", usage.WebSearches)
		}
		result.Extra["模型 "+model] = value
	}
}

func claudeCostDetailLabel(item claudeCostResult) string {
	costType := normalizeClaudeCostTerm(item.CostType)
	tokenType := normalizeClaudeCostTerm(item.TokenType)
	description := strings.ToLower(strings.TrimSpace(item.Description))

	label := ""
	switch {
	case costType == "web_search" || strings.Contains(description, "web search"):
		label = "Web 搜索"
	case costType == "code_execution" || strings.Contains(description, "code execution"):
		label = "代码执行"
	case strings.Contains(tokenType, "cache_read") || strings.Contains(description, "cache read"):
		label = "缓存读取令牌"
	case strings.Contains(tokenType, "1h") || strings.Contains(description, "1h cache"):
		label = "1 小时缓存写入令牌"
	case strings.Contains(tokenType, "5m") || strings.Contains(description, "5m cache"):
		label = "5 分钟缓存写入令牌"
	case strings.Contains(tokenType, "output") || strings.Contains(description, "output token"):
		label = "输出令牌"
	case strings.Contains(tokenType, "input") || strings.Contains(description, "input token"):
		label = "输入令牌"
	case costType == "tokens" || costType == "token" || strings.Contains(description, "token"):
		label = "令牌"
	default:
		label = "其他费用"
	}

	if model := strings.TrimSpace(item.Model); model != "" && strings.Contains(label, "令牌") {
		return model + " · " + label
	}
	return label
}

func normalizeClaudeCostTerm(value string) string {
	return strings.NewReplacer("-", "_", " ", "_").Replace(strings.ToLower(strings.TrimSpace(value)))
}

func cloneURLValues(values url.Values) url.Values {
	cloned := make(url.Values, len(values))
	for key, items := range values {
		cloned[key] = append([]string(nil), items...)
	}
	return cloned
}
