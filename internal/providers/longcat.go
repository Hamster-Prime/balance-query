package providers

import (
	"fmt"
	"time"

	"github.com/Hamster-Prime/balance-query/internal/balance"
)

// Longcat queries the Longcat (meituan) API platform.
// Docs: https://longcat.chat/platform/docs
type Longcat struct{}

type longcatUserResp struct {
	Data struct {
		AvailableBalance float64 `json:"available_balance"`
		UsedBalance      float64 `json:"used_balance"`
		TokensRemaining  int64   `json:"tokens_remaining"`
		TokensTotal      int64   `json:"tokens_total"`
		Plan             string  `json:"plan"`
		ExpireAt         string  `json:"expire_at"`
	} `json:"data"`
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

func (Longcat) Fetch(authID, token, proxyURL string) balance.Result {
	label := balance.ProviderLabel[balance.ProviderLongcat]
	var resp longcatUserResp
	if err := getJSON("https://longcat.chat/platform/api/v1/user/me", token, proxyURL, &resp); err != nil {
		return errResult(authID, label, err.Error())
	}
	d := resp.Data
	display := ""
	if d.TokensTotal > 0 {
		display = fmt.Sprintf("令牌套餐：剩余 %d / %d", d.TokensRemaining, d.TokensTotal)
	} else if d.AvailableBalance > 0 || d.UsedBalance > 0 {
		display = fmt.Sprintf("余额 ¥%.4f（已使用 ¥%.4f）", d.AvailableBalance, d.UsedBalance)
	}
	return balance.Result{
		Provider:        label,
		AuthID:          authID,
		BalanceUSD:      d.AvailableBalance,
		TokensTotal:     d.TokensTotal,
		TokensRemaining: d.TokensRemaining,
		Plan:            d.Plan,
		ResetAt:         d.ExpireAt,
		QuotaDisplay:    display,
		FetchedAt:       time.Now(),
	}
}
