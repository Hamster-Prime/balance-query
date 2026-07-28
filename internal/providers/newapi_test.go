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
	usage.Data.TotalUsed = 125
	usage.HasUsedField = true
	result := parseNewAPITokenUsage("new-api", usage, newAPIStatusResp{})
	if len(result.QuotaWindows) != 1 || !result.QuotaWindows[0].Unlimited {
		t.Fatalf("unlimited result = %#v", result)
	}
	if result.QuotaWindows[0].Used != 125 {
		t.Fatalf("unlimited used quota = %#v", result.QuotaWindows[0])
	}
	if !result.QuotaWindows[0].ShowUsedWhenUnlimited {
		t.Fatalf("unlimited window does not request visible usage: %#v", result.QuotaWindows[0])
	}
	if result.QuotaDisplay != "不限量，已用 125.0000 内部额度" {
		t.Fatalf("quota display = %q", result.QuotaDisplay)
	}
}

func TestParseNewAPITokenUsageUnlimitedZeroUsedClearsSyntheticLimits(t *testing.T) {
	var usage newAPITokenUsageResp
	usage.Code = true
	usage.Data.UnlimitedQuota = true
	usage.HasUsedField = true
	usage.Data.TotalGranted = -100
	usage.Data.TotalAvailable = -100

	result := parseNewAPITokenUsage("new-api", usage, newAPIStatusResp{})
	window := result.QuotaWindows[0]
	if !window.Unlimited || !window.ShowUsedWhenUnlimited || window.Used != 0 {
		t.Fatalf("unlimited zero-used window = %#v", window)
	}
	if window.Total != 0 || window.Remaining != 0 {
		t.Fatalf("unlimited synthetic limits were retained: %#v", window)
	}
	if result.QuotaDisplay != "不限量，已用 0.0000 内部额度" {
		t.Fatalf("quota display = %q", result.QuotaDisplay)
	}
}

func TestNewAPIQuotaConverterHonorsLegacyRawQuotaDisplay(t *testing.T) {
	displayInCurrency := false
	var status newAPIStatusResp
	status.Success = true
	status.Data.QuotaPerUnit = 500_000
	status.Data.DisplayInCurrency = &displayInCurrency
	convert, unit := newAPIQuotaConverter(status)
	if got := convert(1_250_000); got != 1_250_000 || unit != "内部额度" {
		t.Fatalf("legacy raw converter = (%v, %q)", got, unit)
	}
}

func TestNewAPIQuotaConverterFallsBackToInternalUnits(t *testing.T) {
	convert, unit := newAPIQuotaConverter(newAPIStatusResp{})
	if got := convert(123); got != 123 || unit != "内部额度" {
		t.Fatalf("fallback converter = (%v, %q)", got, unit)
	}
}

func TestNewAPIQuotaConverterRejectsIncompleteOrUnknownCurrency(t *testing.T) {
	tests := []struct {
		name        string
		displayType string
	}{
		{name: "CNY without exchange rate", displayType: "CNY"},
		{name: "custom without exchange rate", displayType: "CUSTOM"},
		{name: "unknown display type", displayType: "POINTS"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var status newAPIStatusResp
			status.Success = true
			status.Data.QuotaPerUnit = 500_000
			status.Data.QuotaDisplayType = test.displayType
			convert, unit := newAPIQuotaConverter(status)
			if got := convert(1_250_000); got != 1_250_000 || unit != "内部额度" {
				t.Fatalf("converter = (%v, %q)", got, unit)
			}
		})
	}
}
