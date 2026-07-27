package providers

import "github.com/Hamster-Prime/balance-query/internal/balance"

// XiaomiAPI represents MiMo pay-as-you-go inference keys. The public model API
// key (sk-...) cannot authenticate the web console's /api/v1/balance endpoint;
// that endpoint requires a browser login cookie. Do not send model keys there.
type XiaomiAPI struct{}

func (XiaomiAPI) Fetch(authID, _, _ string) balance.Result {
	return errResult(authID, balance.ProviderLabel[balance.ProviderXiaomiAPI],
		"小米官方未提供模型 API Key 的余额查询接口；按量余额只能在 MiMo 控制台登录后查看")
}

// XiaomiTokenPlan represents MiMo Token Plan keys (tp-...). The documented
// inference key cannot access tokenPlan/detail or tokenPlan/usage because both
// are browser-session console APIs.
type XiaomiTokenPlan struct{}

func (XiaomiTokenPlan) Fetch(authID, _, _ string) balance.Result {
	return errResult(authID, balance.ProviderLabel[balance.ProviderXiaomiToken],
		"小米 Token Plan 的套餐明细仅由官网登录会话提供，tp- 模型密钥不能查询；请在订阅管理页查看 Credits 用量和周期")
}
