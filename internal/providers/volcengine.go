package providers

import "github.com/Hamster-Prime/balance-query/internal/balance"

// Volcengine documents the Coding Plan inference base URL and API key, but no
// API-key-authenticated quota endpoint. The old /user/coding_plan/quota path was
// inferred and is deliberately no longer called.
type VolcengineCodingPlan struct{}

func (VolcengineCodingPlan) Fetch(authID, _, _ string) balance.Result {
	return errResult(authID, balance.ProviderLabel[balance.ProviderVolcengine],
		newProviderError(balance.FailureUnsupported,
			"火山引擎官方尚未公开 Coding Plan API Key 的配额查询接口，请在方舟控制台查看套餐用量", 0, "", nil))
}
