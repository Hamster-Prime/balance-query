package providers

import (
	"fmt"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/examples/plugin/balance-query/go/internal/balance"
)

// NewAPI handles New API / One API compatible relay instances.
// BaseURL is required and must be set per-auth by the user.
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

func (n NewAPI) Fetch(authID, token string) balance.Result {
	label := balance.ProviderLabel[balance.ProviderNewAPI]
	baseURL := n.BaseURL
	if baseURL == "" {
		return errResult(authID, label,
			"New API base URL not configured — please set it in the Balance Query settings page")
	}
	var resp newAPIUserSelfResp
	if err := getJSON(baseURL+"/api/user/self", token, &resp); err != nil {
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
		QuotaDisplay: fmt.Sprintf("$%.4f 剩余 (已用 $%.4f)", remainUSD, usedUSD),
		Extra:        map[string]string{"username": resp.Data.Username},
		FetchedAt:    time.Now(),
	}
}
