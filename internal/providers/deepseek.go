package providers

import (
	"fmt"
	"time"

	"github.com/Hamster-Prime/balance-query/internal/balance"
)

// DeepSeek queries https://api.deepseek.com/user/balance.
// Docs: https://api-docs.deepseek.com/api/get-user-balance
type DeepSeek struct{}

type deepSeekResp struct {
	IsAvailable  bool `json:"is_available"`
	BalanceInfos []struct {
		Currency        string `json:"currency"`
		TotalBalance    string `json:"total_balance"`
		GrantedBalance  string `json:"granted_balance"`
		ToppedUpBalance string `json:"topped_up_balance"`
	} `json:"balance_infos"`
}

func (DeepSeek) Fetch(authID, token, proxyURL string) balance.Result {
	var resp deepSeekResp
	if err := getJSON("https://api.deepseek.com/user/balance", token, proxyURL, &resp); err != nil {
		return errResult(authID, balance.ProviderLabel[balance.ProviderDeepSeek], err.Error())
	}
	r := balance.Result{
		Provider:  balance.ProviderLabel[balance.ProviderDeepSeek],
		AuthID:    authID,
		FetchedAt: time.Now(),
		Extra:     map[string]string{"账户状态": map[bool]string{true: "可用", false: "不可用"}[resp.IsAvailable]},
	}
	for _, b := range resp.BalanceInfos {
		switch b.Currency {
		case "CNY":
			r.QuotaDisplay = fmt.Sprintf("¥%s（赠送 ¥%s，充值 ¥%s）",
				b.TotalBalance, b.GrantedBalance, b.ToppedUpBalance)
		case "USD":
			r.QuotaDisplay = fmt.Sprintf("$%s（赠送 $%s，充值 $%s）",
				b.TotalBalance, b.GrantedBalance, b.ToppedUpBalance)
		}
	}
	return r
}
