package providers

import (
	"fmt"
	"time"

	"github.com/Hamster-Prime/balance-query/internal/balance"
)

// Sub2API queries a Sub2API instance's usage via GET /v1/usage.
// The API key is a group API key that exposes accounting data.
// BaseURL comes from the selected CPA OpenAI-compatible provider.
// Source: CodexBar docs (GET /v1/usage endpoint).
type Sub2API struct {
	BaseURL string // e.g. "https://your-sub2api.example.com"
}

type sub2APIUsageResp struct {
	Data struct {
		Plan            string `json:"plan"`
		TokensTotal     int64  `json:"tokens_total"`
		TokensUsed      int64  `json:"tokens_used"`
		TokensRemaining int64  `json:"tokens_remaining"`
		ResetAt         string `json:"reset_at"`
		// Legacy one-api style quota fields (unit = 1 USD / 500000)
		Quota     float64 `json:"quota"`
		UsedQuota float64 `json:"used_quota"`
	} `json:"data"`
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func (s Sub2API) Fetch(authID, token, proxyURL string) balance.Result {
	baseURL := s.BaseURL
	if baseURL == "" {
		return errResult(authID, balance.ProviderLabel[balance.ProviderSub2API],
			"所选 OpenAI 兼容提供商没有配置接口地址")
	}
	endpoint, err := serviceEndpoint(baseURL, "/v1/usage")
	if err != nil {
		return errResult(authID, balance.ProviderLabel[balance.ProviderSub2API], err.Error())
	}
	var resp sub2APIUsageResp
	if err := getJSON(endpoint, token, proxyURL, &resp); err != nil {
		return errResult(authID, balance.ProviderLabel[balance.ProviderSub2API], err.Error())
	}
	d := resp.Data
	remaining := d.TokensRemaining
	if remaining == 0 && d.TokensTotal > 0 {
		remaining = d.TokensTotal - d.TokensUsed
	}
	display := ""
	if d.TokensTotal > 0 {
		display = fmt.Sprintf("已使用 %d / %d 令牌", d.TokensUsed, d.TokensTotal)
	} else if d.Quota > 0 {
		const u = 500000.0
		display = fmt.Sprintf("剩余 $%.4f（已使用 $%.4f）", d.Quota/u, d.UsedQuota/u)
	}
	return balance.Result{
		Provider:        balance.ProviderLabel[balance.ProviderSub2API],
		AuthID:          authID,
		TokensTotal:     d.TokensTotal,
		TokensUsed:      d.TokensUsed,
		TokensRemaining: remaining,
		Plan:            d.Plan,
		ResetAt:         d.ResetAt,
		QuotaDisplay:    display,
		FetchedAt:       time.Now(),
	}
}
