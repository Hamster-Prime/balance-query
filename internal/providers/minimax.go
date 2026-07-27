package providers

import (
	"fmt"
	"time"

	"github.com/Hamster-Prime/balance-query/internal/balance"
)

// MiniMaxAPI queries MiniMax pay-as-you-go balance (国内, api.minimaxi.com).
type MiniMaxAPI struct{}

type miniMaxBalanceResp struct {
	Data struct {
		AvailableCredits float64 `json:"available_credits"`
		UsedCredits      float64 `json:"used_credits"`
	} `json:"data"`
	BaseResp struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp"`
}

func (MiniMaxAPI) Fetch(authID, token string) balance.Result {
	label := balance.ProviderLabel[balance.ProviderMiniMaxAPI]
	var resp miniMaxBalanceResp
	if err := getJSON("https://api.minimaxi.com/v1/user/balance", token, &resp); err != nil {
		return errResult(authID, label, err.Error())
	}
	if resp.BaseResp.StatusCode != 0 {
		return errResult(authID, label, resp.BaseResp.StatusMsg)
	}
	d := resp.Data
	return balance.Result{
		Provider:     label,
		AuthID:       authID,
		BalanceUSD:   d.AvailableCredits,
		QuotaDisplay: fmt.Sprintf("¥%.4f 可用 (已用 ¥%.4f)", d.AvailableCredits, d.UsedCredits),
		FetchedAt:    time.Now(),
	}
}

// ── MiniMax Coding Plan ──────────────────────────────────────────────────────

type miniMaxCodingResp struct {
	Data struct {
		TokensRemaining int64  `json:"tokens_remaining"`
		TokensTotal     int64  `json:"tokens_total"`
		TokensUsed      int64  `json:"tokens_used"`
		Plan            string `json:"plan"`
		ResetAt         string `json:"reset_at"`
	} `json:"data"`
	BaseResp struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp"`
}

func fetchMiniMaxCoding(authID, token, apiBase, label string) balance.Result {
	var resp miniMaxCodingResp
	if err := getJSON(apiBase+"/v1/token-plan/quota", token, &resp); err != nil {
		return errResult(authID, label, err.Error())
	}
	if resp.BaseResp.StatusCode != 0 {
		return errResult(authID, label, resp.BaseResp.StatusMsg)
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
		QuotaDisplay:    fmt.Sprintf("%d / %d tokens 剩余", remaining, d.TokensTotal),
		FetchedAt:       time.Now(),
	}
}

// MiniMaxCodingCN queries MiniMax Token Plan quota (国内, api.minimaxi.com).
type MiniMaxCodingCN struct{}

func (MiniMaxCodingCN) Fetch(authID, token string) balance.Result {
	return fetchMiniMaxCoding(authID, token, "https://api.minimaxi.com",
		balance.ProviderLabel[balance.ProviderMiniMaxCodingCN])
}

// MiniMaxCodingGlobal queries MiniMax Token Plan quota (海外, api.minimax.io).
type MiniMaxCodingGlobal struct{}

func (MiniMaxCodingGlobal) Fetch(authID, token string) balance.Result {
	return fetchMiniMaxCoding(authID, token, "https://api.minimax.io",
		balance.ProviderLabel[balance.ProviderMiniMaxCodingGlobal])
}
