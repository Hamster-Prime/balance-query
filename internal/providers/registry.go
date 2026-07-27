package providers

import (
	"github.com/Hamster-Prime/balance-query/internal/balance"
)

// Build returns a Fetcher for the given ProviderType. baseURL is inherited
// from the selected CPA AI provider and is used by self-hosted services and
// official endpoints that expose account data.
func Build(p balance.ProviderType, baseURL string) balance.Fetcher {
	switch p {
	case balance.ProviderSub2API:
		return Sub2API{BaseURL: baseURL}
	case balance.ProviderClaudeAdmin:
		return ClaudeAdmin{BaseURL: baseURL}
	case balance.ProviderDeepSeek:
		return DeepSeek{BaseURL: baseURL}
	case balance.ProviderGLMZAI:
		return GLMZai{}
	case balance.ProviderGLMZhipu:
		return GLMZhipu{}
	case balance.ProviderNewAPI:
		return NewAPI{BaseURL: baseURL}
	case balance.ProviderKimiAPI:
		return KimiAPI{BaseURL: baseURL}
	case balance.ProviderKimiCode:
		return KimiCode{}
	case balance.ProviderLongcat:
		return Longcat{}
	case balance.ProviderMiniMaxAPI:
		return MiniMaxAPI{}
	case balance.ProviderMiniMaxCodingCN:
		return MiniMaxCodingCN{}
	case balance.ProviderMiniMaxCodingGlobal:
		return MiniMaxCodingGlobal{}
	case balance.ProviderOpenCode:
		return OpenCode{}
	case balance.ProviderVolcengine:
		return VolcengineCodingPlan{}
	case balance.ProviderXiaomiAPI:
		return XiaomiAPI{}
	case balance.ProviderXiaomiToken:
		return XiaomiTokenPlan{}
	default:
		return nil
	}
}
