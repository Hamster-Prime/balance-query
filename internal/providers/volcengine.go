package providers

import (
	"fmt"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/examples/plugin/balance-query/go/internal/balance"
)

// VolcengineCodingPlan queries the Volcengine (火山引擎) Ark Coding Plan quota.
// API base: https://ark.cn-beijing.volces.com/api/v3
type VolcengineCodingPlan struct{}

type volcengineQuotaResp struct {
	Data struct {
		TokensRemaining int64  `json:"tokens_remaining"`
		TokensTotal     int64  `json:"tokens_total"`
		TokensUsed      int64  `json:"tokens_used"`
		Plan            string `json:"plan"`
		ExpireAt        string `json:"expire_at"`
	} `json:"data"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (VolcengineCodingPlan) Fetch(authID, token string) balance.Result {
	label := balance.ProviderLabel[balance.ProviderVolcengine]
	var resp volcengineQuotaResp
	if err := getJSON("https://ark.cn-beijing.volces.com/api/v3/user/coding_plan/quota", token, &resp); err != nil {
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
		ResetAt:         d.ExpireAt,
		QuotaDisplay:    fmt.Sprintf("%d / %d tokens 剩余", remaining, d.TokensTotal),
		FetchedAt:       time.Now(),
	}
}
