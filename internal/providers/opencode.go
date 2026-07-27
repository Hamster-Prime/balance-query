package providers

import "github.com/Hamster-Prime/balance-query/internal/balance"

// OpenCode's public inference key does not have a documented balance endpoint.
// Workspace billing exists only behind the authenticated console/RPC layer.
type OpenCode struct{}

func (OpenCode) Fetch(authID, _, _ string) balance.Result {
	return errResult(authID, balance.ProviderLabel[balance.ProviderOpenCode],
		"OpenCode 官方未提供模型 API Key 可访问的余额接口；工作区账单只能在登录控制台中查看")
}
