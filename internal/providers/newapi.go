package providers

import (
	"fmt"
	"time"

	"github.com/Hamster-Prime/balance-query/internal/balance"
)

// NewAPI handles New API / One API compatible relay instances.
// BaseURL comes from the selected CPA OpenAI-compatible provider.
// Endpoint: GET <baseURL>/api/user/self
type NewAPI struct {
	BaseURL string // e.g. "https://api.newapi.ai"
}

type newAPIUserSelfResp struct {
	Success bool `json:"success"`
	Data    struct {
		Username  string  `json:"username"`
		Quota     float64 `json:"quota"`      // remaining, unit: 1 USD = 500000
		UsedQuota float64 `json:"used_quota"` // used, same unit
	} `json:"data"`
	Message string `json:"message"`
}

func (n NewAPI) Fetch(authID, token, proxyURL string) balance.Result {
	label := balance.ProviderLabel[balance.ProviderNewAPI]
	baseURL := n.BaseURL
	if baseURL == "" {
		return errResult(authID, label, "所选 OpenAI 兼容提供商没有配置接口地址")
	}
	endpoint, err := serviceEndpoint(baseURL, "/api/user/self")
	if err != nil {
		return errResult(authID, label, err.Error())
	}
	var resp newAPIUserSelfResp
	if err := getJSON(endpoint, token, proxyURL, &resp); err != nil {
		return errResult(authID, label, err.Error())
	}
	if !resp.Success {
		return errResult(authID, label, resp.Message)
	}
	const unitPerDollar = 500000.0
	remainUSD := resp.Data.Quota / unitPerDollar
	usedUSD := resp.Data.UsedQuota / unitPerDollar
	return balance.Result{
		Provider:     label,
		AuthID:       authID,
		BalanceUSD:   remainUSD,
		QuotaDisplay: fmt.Sprintf("剩余 $%.4f（已使用 $%.4f）", remainUSD, usedUSD),
		Extra:        map[string]string{"用户名": resp.Data.Username},
		FetchedAt:    time.Now(),
	}
}
