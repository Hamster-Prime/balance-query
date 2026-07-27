package providers

import (
	"fmt"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/examples/plugin/balance-query/go/internal/balance"
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

func (DeepSeek) Fetch(authID, token string) balance.Result {
	var resp deepSeekResp
	if err := getJSON("https://api.deepseek.com/user/balance", token, &resp); err != nil {
		return errResult(authID, balance.ProviderLabel[balance.ProviderDeepSeek], err.Error())
	}
	r := balance.Result{
		Provider:  balance.ProviderLabel[balance.ProviderDeepSeek],
		AuthID:    authID,
		FetchedAt: time.Now(),
		Extra:     map[string]string{"available": fmt.Sprintf("%v", resp.IsAvailable)},
	}
	for _, b := range resp.BalanceInfos {
		switch b.Currency {
		case "CNY":
			r.QuotaDisplay = fmt.Sprintf("¥%s (赠: ¥%s, 充: ¥%s)",
				b.TotalBalance, b.GrantedBalance, b.ToppedUpBalance)
		case "USD":
			r.QuotaDisplay = fmt.Sprintf("$%s (granted: $%s, topped-up: $%s)",
				b.TotalBalance, b.GrantedBalance, b.ToppedUpBalance)
		}
	}
	return r
}
