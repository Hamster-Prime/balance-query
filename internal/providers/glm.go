package providers

import (
	"fmt"
	"time"

	"github.com/Hamster-Prime/balance-query/internal/balance"
)

// GLMZai queries the Z.AI platform GLM Coding Plan quota.
// Endpoint: GET https://api.z.ai/api/monitor/usage/quota/limit
// Auth: Authorization header WITHOUT "Bearer" prefix (raw token).
// Source: https://github.com/guyinwonder168/opencode-glm-quota
type GLMZai struct{}

// GLMZhipu queries the Zhipu/bigmodel.cn platform GLM Coding Plan quota.
// Endpoint: GET https://open.bigmodel.cn/api/monitor/usage/quota/limit
// Auth: same — raw token, no "Bearer" prefix.
type GLMZhipu struct{}

type glmQuotaResp struct {
	Data struct {
		Level  string         `json:"level"`
		Limits []glmLimitItem `json:"limits"`
	} `json:"data"`
}

type glmLimitItem struct {
	Type         string  `json:"type"`       // "TOKENS_LIMIT" or "TIME_LIMIT"
	Percentage   float64 `json:"percentage"` // used percentage (0–100)
	Unit         int64   `json:"unit"`
	Number       int64   `json:"number"`
	CurrentValue int64   `json:"currentValue"`
	Total        int64   `json:"total"`
	// Unix ms timestamp for next reset.
	NextResetTime int64 `json:"nextResetTime"`
}

func fetchGLMQuota(authID, token, proxyURL, baseURL, label string) balance.Result {
	url := baseURL + "/api/monitor/usage/quota/limit"
	var resp glmQuotaResp
	// GLM requires raw token — no "Bearer" prefix.
	if err := getJSONRawAuth(url, token, proxyURL, &resp); err != nil {
		return errResult(authID, label, err.Error())
	}
	d := resp.Data

	r := balance.Result{
		Provider:  label,
		AuthID:    authID,
		Plan:      d.Level,
		FetchedAt: time.Now(),
		Extra:     make(map[string]string),
	}
	if d.Level != "" {
		r.Extra["套餐等级"] = d.Level
	}

	for _, lim := range d.Limits {
		switch lim.Type {
		case "TOKENS_LIMIT":
			r.TokensTotal = lim.Number
			used := int64(float64(lim.Number) * lim.Percentage / 100)
			r.TokensUsed = used
			r.TokensRemaining = lim.Number - used
			if lim.NextResetTime > 0 {
				r.ResetAt = fmt.Sprintf("%s", time.UnixMilli(lim.NextResetTime).Format("01-02 15:04"))
			}
			r.QuotaDisplay = fmt.Sprintf("5 小时窗口已使用 %.1f%%（共 %d 令牌）", lim.Percentage, lim.Number)
		case "TIME_LIMIT":
			r.Extra["MCP 月用量"] = fmt.Sprintf("%.1f%%", lim.Percentage)
		}
	}
	return r
}

func (GLMZai) Fetch(authID, token, proxyURL string) balance.Result {
	return fetchGLMQuota(authID, token, proxyURL, "https://api.z.ai", balance.ProviderLabel[balance.ProviderGLMZAI])
}

func (GLMZhipu) Fetch(authID, token, proxyURL string) balance.Result {
	return fetchGLMQuota(authID, token, proxyURL, "https://open.bigmodel.cn", balance.ProviderLabel[balance.ProviderGLMZhipu])
}
