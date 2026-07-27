package providers

import (
	"fmt"
	"time"

	"github.com/Hamster-Prime/balance-query/internal/balance"
)

// OpenCode queries OpenCode Zen pay-as-you-go credit balance.
type OpenCode struct{}

type openCodeBalanceResp struct {
	Data struct {
		AvailableCredits float64 `json:"available_credits"`
		UsedCredits      float64 `json:"used_credits"`
		Plan             string  `json:"plan"`
	} `json:"data"`
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func (OpenCode) Fetch(authID, token string) balance.Result {
	label := balance.ProviderLabel[balance.ProviderOpenCode]
	var resp openCodeBalanceResp
	if err := getJSON("https://opencode.ai/api/v1/user/balance", token, &resp); err != nil {
		return errResult(authID, label, err.Error())
	}
	if !resp.Success {
		return errResult(authID, label, resp.Message)
	}
	d := resp.Data
	return balance.Result{
		Provider:     label,
		AuthID:       authID,
		BalanceUSD:   d.AvailableCredits,
		Plan:         d.Plan,
		QuotaDisplay: fmt.Sprintf("$%.4f 可用 (已用 $%.4f)", d.AvailableCredits, d.UsedCredits),
		FetchedAt:    time.Now(),
	}
}
