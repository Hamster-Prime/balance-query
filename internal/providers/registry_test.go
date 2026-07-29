package providers

import (
	"strings"
	"testing"

	"github.com/Hamster-Prime/balance-query/internal/balance"
)

func TestEveryKnownProviderHasLabelAndFetcher(t *testing.T) {
	seen := make(map[balance.ProviderType]bool)
	for _, providerType := range balance.AllProviders() {
		if seen[providerType] {
			t.Fatalf("provider %q is listed more than once", providerType)
		}
		seen[providerType] = true
		if strings.TrimSpace(balance.ProviderLabel[providerType]) == "" {
			t.Errorf("provider %q has no display label", providerType)
		}
		if fetcher := Build(providerType, "https://provider.example/v1"); fetcher == nil {
			t.Errorf("provider %q has no fetcher", providerType)
		}
	}
	for providerType := range balance.ProviderLabel {
		if !seen[providerType] {
			t.Errorf("label exists for provider %q which is missing from AllProviders", providerType)
		}
	}
}
