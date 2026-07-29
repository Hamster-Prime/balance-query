package providers

import (
	"fmt"
	"strings"
	"time"

	"github.com/Hamster-Prime/balance-query/internal/balance"
)

// DeepSeek queries https://api.deepseek.com/user/balance.
// Docs: https://api-docs.deepseek.com/api/get-user-balance
type DeepSeek struct {
	BaseURL string
}

type deepSeekResp struct {
	IsAvailable  bool `json:"is_available"`
	BalanceInfos []struct {
		Currency        string `json:"currency"`
		TotalBalance    string `json:"total_balance"`
		GrantedBalance  string `json:"granted_balance"`
		ToppedUpBalance string `json:"topped_up_balance"`
	} `json:"balance_infos"`
}

func (d DeepSeek) Fetch(authID, token, proxyURL string) balance.Result {
	endpoint := "https://api.deepseek.com/user/balance"
	if strings.TrimSpace(d.BaseURL) != "" {
		derived, err := serviceEndpoint(d.BaseURL, "/user/balance")
		if err != nil {
			return errResult(authID, balance.ProviderLabel[balance.ProviderDeepSeek], err)
		}
		endpoint = derived
	}
	var resp deepSeekResp
	if err := getJSON(endpoint, token, proxyURL, &resp); err != nil {
		return errResult(authID, balance.ProviderLabel[balance.ProviderDeepSeek], err)
	}
	r := balance.Result{
		Provider:  balance.ProviderLabel[balance.ProviderDeepSeek],
		AuthID:    authID,
		FetchedAt: time.Now(),
		Extra:     map[string]string{"账户状态": map[bool]string{true: "可用", false: "不可用"}[resp.IsAvailable]},
	}
	if len(resp.BalanceInfos) == 0 {
		return errResult(authID, balance.ProviderLabel[balance.ProviderDeepSeek],
			invalidResponseError("官方接口未返回余额明细"))
	}
	for _, b := range resp.BalanceInfos {
		currency := b.Currency
		if currency == "" {
			currency = "未知币种"
		}
		r.Extra[currency+" 总余额"] = b.TotalBalance
		r.Extra[currency+" 赠送余额"] = b.GrantedBalance
		r.Extra[currency+" 充值余额"] = b.ToppedUpBalance
		switch b.Currency {
		case "CNY":
			amount, ok := numberValue(b.TotalBalance)
			if !ok {
				return errResult(authID, balance.ProviderLabel[balance.ProviderDeepSeek],
					invalidResponseError("DeepSeek 余额接口返回了无效的人民币余额"))
			}
			if !r.HasBalanceAmount {
				r.BalanceAmount = amount
				r.BalanceCurrency = "CNY"
				r.HasBalanceAmount = true
				r.BalanceScope = "account"
			}
			if r.QuotaDisplay == "" {
				r.QuotaDisplay = fmt.Sprintf("可用 ¥%s", b.TotalBalance)
			}
		case "USD":
			amount, ok := numberValue(b.TotalBalance)
			if !ok {
				return errResult(authID, balance.ProviderLabel[balance.ProviderDeepSeek],
					invalidResponseError("DeepSeek 余额接口返回了无效的美元余额"))
			}
			if r.QuotaDisplay == "" {
				r.QuotaDisplay = fmt.Sprintf("可用 $%s", b.TotalBalance)
			}
			if !r.HasBalanceAmount {
				r.BalanceAmount = amount
				r.BalanceCurrency = "USD"
				r.HasBalanceAmount = true
			}
			r.BalanceUSD = amount
			r.HasBalance = true
			r.BalanceScope = "account"
		}
	}
	if r.QuotaDisplay == "" {
		first := resp.BalanceInfos[0]
		r.QuotaDisplay = fmt.Sprintf("可用 %s %s", first.TotalBalance, first.Currency)
	}
	return r
}
