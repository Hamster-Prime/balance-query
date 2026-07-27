package providers

import (
	"fmt"
	"time"

	"github.com/Hamster-Prime/balance-query/internal/balance"
)

// XiaomiAPI queries the Xiaomi MiMo pay-as-you-go API balance.
// Base URL: https://platform.xiaomimimo.com/api/v1
type XiaomiAPI struct{}

type xiaomiBalanceResp struct {
	Data struct {
		AvailableBalance float64 `json:"available_balance"`
		UsedBalance      float64 `json:"used_balance"`
		TotalBalance     float64 `json:"total_balance"`
	} `json:"data"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (XiaomiAPI) Fetch(authID, token, proxyURL string) balance.Result {
	label := balance.ProviderLabel[balance.ProviderXiaomiAPI]
	var resp xiaomiBalanceResp
	if err := getJSON("https://platform.xiaomimimo.com/api/v1/user/balance", token, proxyURL, &resp); err != nil {
		return errResult(authID, label, err.Error())
	}
	d := resp.Data
	return balance.Result{
		Provider:     label,
		AuthID:       authID,
		BalanceUSD:   d.AvailableBalance,
		QuotaDisplay: fmt.Sprintf("可用 ¥%.4f（已使用 ¥%.4f，共 ¥%.4f）", d.AvailableBalance, d.UsedBalance, d.TotalBalance),
		FetchedAt:    time.Now(),
	}
}

// XiaomiTokenPlan queries the Xiaomi MiMo Token Plan subscription quota.
type XiaomiTokenPlan struct{}

type xiaomiTokenPlanResp struct {
	Data struct {
		TokensRemaining int64  `json:"tokens_remaining"`
		TokensTotal     int64  `json:"tokens_total"`
		TokensUsed      int64  `json:"tokens_used"`
		Plan            string `json:"plan"`
		ResetAt         string `json:"reset_at"`
	} `json:"data"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (XiaomiTokenPlan) Fetch(authID, token, proxyURL string) balance.Result {
	label := balance.ProviderLabel[balance.ProviderXiaomiToken]
	var resp xiaomiTokenPlanResp
	if err := getJSON("https://platform.xiaomimimo.com/api/v1/token-plan/quota", token, proxyURL, &resp); err != nil {
		return errResult(authID, label, err.Error())
	}
	d := resp.Data
	remaining := d.TokensRemaining
	if remaining == 0 {
		remaining = d.TokensTotal - d.TokensUsed
	}
	return balance.Result{
		Provider:        label,
		AuthID:          authID,
		TokensTotal:     d.TokensTotal,
		TokensUsed:      d.TokensUsed,
		TokensRemaining: remaining,
		Plan:            d.Plan,
		ResetAt:         d.ResetAt,
		QuotaDisplay:    fmt.Sprintf("剩余 %d / %d 令牌", remaining, d.TokensTotal),
		FetchedAt:       time.Now(),
	}
}
