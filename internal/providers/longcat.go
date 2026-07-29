package providers

import "github.com/Hamster-Prime/balance-query/internal/balance"

// Longcat model API keys only authenticate inference endpoints. The official
// PAYG and Token Pack summary endpoints are console APIs requiring a LongCat
// browser session cookie, so a CPA OpenAI-compatible provider key cannot query
// them safely.
type Longcat struct{}

func (Longcat) Fetch(authID, _, _ string) balance.Result {
	return errResult(authID, balance.ProviderLabel[balance.ProviderLongcat],
		newProviderError(balance.FailureUnsupported,
			"LongCat 官方未提供模型 API Key 的余额/Token Pack 查询接口；余额、多个 30 天 Token 包及到期时间只能在官网登录控制台查看", 0, "", nil))
}
