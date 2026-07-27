package providers

import "testing"

func TestParseNewAPITokenUsageWithSiteCurrency(t *testing.T) {
	var usage newAPITokenUsageResp
	usage.Code = true
	usage.Data.Name = "生产密钥"
	usage.Data.TotalGranted = 5_000_000
	usage.Data.TotalUsed = 1_250_000
	usage.Data.TotalAvailable = 3_750_000
	usage.Data.ModelLimitsEnabled = true
	usage.Data.ModelLimits = map[string]bool{"gpt-4o": true, "gpt-4o-mini": true}
	usage.Data.ExpiresAt = 1_800_000_000

	var status newAPIStatusResp
	status.Success = true
	status.Data.QuotaPerUnit = 500_000
	status.Data.QuotaDisplayType = "CNY"
	status.Data.USDExchangeRate = 7.2

	result := parseNewAPITokenUsage("new-api", usage, status)
	if len(result.QuotaWindows) != 1 {
		t.Fatalf("quota windows = %d, want 1", len(result.QuotaWindows))
	}
	window := result.QuotaWindows[0]
	if window.Total != 72 || window.Used != 18 || window.Remaining != 54 || window.Unit != "CNY" {
		t.Fatalf("converted window = %#v", window)
	}
	if result.Extra["密钥名称"] != "生产密钥" {
		t.Fatalf("key name = %q", result.Extra["密钥名称"])
	}
	if result.Extra["允许模型"] != "gpt-4o、gpt-4o-mini" {
		t.Fatalf("model limits = %q", result.Extra["允许模型"])
	}
}

func TestParseNewAPITokenUsageUnlimited(t *testing.T) {
	var usage newAPITokenUsageResp
	usage.Code = true
	usage.Data.UnlimitedQuota = true
	result := parseNewAPITokenUsage("new-api", usage, newAPIStatusResp{})
	if len(result.QuotaWindows) != 1 || !result.QuotaWindows[0].Unlimited {
		t.Fatalf("unlimited result = %#v", result)
	}
	if result.QuotaDisplay != "密钥额度不设上限" {
		t.Fatalf("quota display = %q", result.QuotaDisplay)
	}
}

func TestNewAPIQuotaConverterFallsBackToInternalUnits(t *testing.T) {
	convert, unit := newAPIQuotaConverter(newAPIStatusResp{})
	if got := convert(123); got != 123 || unit != "内部额度" {
		t.Fatalf("fallback converter = (%v, %q)", got, unit)
	}
}
