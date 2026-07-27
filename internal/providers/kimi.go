package providers

import (
	"fmt"
	"time"

	"github.com/Hamster-Prime/balance-query/internal/balance"
)

// KimiAPI queries the official Moonshot/Kimi pay-as-you-go API balance.
// Docs: https://platform.kimi.ai/docs/api/balance
type KimiAPI struct{}

type kimiBalanceResp struct {
	Code   int  `json:"code"`
	Status bool `json:"status"`
	Data   struct {
		AvailableBalance float64 `json:"available_balance"`
		VoucherBalance   float64 `json:"voucher_balance"`
		CashBalance      float64 `json:"cash_balance"`
	} `json:"data"`
}

func (KimiAPI) Fetch(authID, token string) balance.Result {
	var resp kimiBalanceResp
	if err := getJSON("https://api.moonshot.ai/v1/users/me/balance", token, &resp); err != nil {
		return errResult(authID, balance.ProviderLabel[balance.ProviderKimiAPI], err.Error())
	}
	return balance.Result{
		Provider: balance.ProviderLabel[balance.ProviderKimiAPI],
		AuthID:   authID,
		QuotaDisplay: fmt.Sprintf("可用: %.4f 元 (现金: %.4f, 赠金: %.4f)",
			resp.Data.AvailableBalance, resp.Data.CashBalance, resp.Data.VoucherBalance),
		FetchedAt: time.Now(),
	}
}

// ── Kimi Coding Plan ─────────────────────────────────────────────────────────

// KimiCode queries the Kimi Code subscription quota.
// Endpoint: GET https://api.kimi.com/coding/v1/usages
// Auth: Bearer token (Kimi Code API key from console).
// Source: https://github.com/slkiser/opencode-quota
type KimiCode struct{}

type kimiUsageResp struct {
	Data struct {
		Usage struct {
			Limit     int64   `json:"limit"`
			Used      int64   `json:"used"`
			Remaining int64   `json:"remaining"`
			Name      string  `json:"name"`
			ResetAt   string  `json:"reset_at"`
			ResetIn   float64 `json:"reset_in"` // seconds until reset
		} `json:"usage"`
		Limits []struct {
			Window struct {
				Duration int64  `json:"duration"`
				TimeUnit string `json:"timeUnit"`
			} `json:"window"`
			Detail struct {
				Limit     int64   `json:"limit"`
				Used      int64   `json:"used"`
				Remaining int64   `json:"remaining"`
				Name      string  `json:"name"`
				ResetAt   string  `json:"reset_at"`
				ResetIn   float64 `json:"reset_in"`
			} `json:"detail"`
		} `json:"limits"`
	} `json:"data"`
}

func (KimiCode) Fetch(authID, token string) balance.Result {
	var resp kimiUsageResp
	if err := getJSON("https://api.kimi.com/coding/v1/usages", token, &resp); err != nil {
		return errResult(authID, balance.ProviderLabel[balance.ProviderKimiCode], err.Error())
	}

	label := balance.ProviderLabel[balance.ProviderKimiCode]
	r := balance.Result{
		Provider:  label,
		AuthID:    authID,
		FetchedAt: time.Now(),
	}

	// Primary usage window (weekly).
	u := resp.Data.Usage
	if u.Limit > 0 {
		r.TokensTotal = u.Limit
		r.TokensUsed = u.Used
		r.TokensRemaining = u.Limit - u.Used
		if u.Remaining > 0 {
			r.TokensRemaining = u.Remaining
		}
		r.ResetAt = u.ResetAt
		if u.ResetAt == "" && u.ResetIn > 0 {
			r.ResetAt = fmt.Sprintf("%.0f 小时后", u.ResetIn/3600)
		}
		r.QuotaDisplay = fmt.Sprintf("%d / %d tokens 已用", u.Used, u.Limit)
	}

	// Annotate sub-windows (5h etc.) as extra.
	if len(resp.Data.Limits) > 0 && r.Extra == nil {
		r.Extra = make(map[string]string)
	}
	for _, lim := range resp.Data.Limits {
		d := lim.Detail
		if d.Limit == 0 {
			continue
		}
		windowLabel := ""
		w := lim.Window
		switch {
		case w.TimeUnit == "MINUTE" && w.Duration == 300:
			windowLabel = "5h窗口"
		case w.TimeUnit == "HOUR":
			windowLabel = fmt.Sprintf("%dh窗口", w.Duration)
		case w.TimeUnit == "DAY":
			windowLabel = fmt.Sprintf("%dd窗口", w.Duration)
		default:
			if d.Name != "" {
				windowLabel = d.Name
			} else {
				continue
			}
		}
		r.Extra[windowLabel] = fmt.Sprintf("%d / %d", d.Used, d.Limit)
	}

	return r
}
