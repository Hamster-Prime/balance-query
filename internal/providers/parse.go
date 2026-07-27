package providers

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

func numberValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return 0, false
		}
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
	case jsonNumber:
		parsed, err := strconv.ParseFloat(string(typed), 64)
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func formatQuotaNumber(value float64) string {
	if math.Abs(value-math.Round(value)) < 1e-9 {
		return strconv.FormatFloat(value, 'f', 0, 64)
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

// jsonNumber is kept as a local alias so helpers can also be exercised with
// plain strings without coupling provider structs to encoding/json.Number.
type jsonNumber string

func int64Value(value any) (int64, bool) {
	number, ok := numberValue(value)
	if !ok {
		return 0, false
	}
	return int64(number), true
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func firstValue(values map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, exists := values[key]; exists && value != nil {
			return value
		}
	}
	return nil
}

func firstNumber(values map[string]any, keys ...string) (float64, bool) {
	return numberValue(firstValue(values, keys...))
}

func firstString(values map[string]any, keys ...string) string {
	return stringValue(firstValue(values, keys...))
}

func clampPercent(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func percentFromValues(used, total float64) float64 {
	if total <= 0 {
		return 0
	}
	return clampPercent(used / total * 100)
}

func formatUnixTimestamp(value int64) string {
	if value <= 0 {
		return ""
	}
	// Contemporary millisecond timestamps are much larger than seconds.
	if value > 100_000_000_000 {
		return time.UnixMilli(value).Local().Format("2006-01-02 15:04")
	}
	return time.Unix(value, 0).Local().Format("2006-01-02 15:04")
}

// addToTimestamp converts the timestamp shapes used by provider responses and
// returns the end of a fixed-duration window. JSON APIs in this project use a
// mix of RFC3339 strings and Unix seconds/milliseconds.
func addToTimestamp(value any, duration time.Duration) string {
	if text, ok := value.(string); ok {
		text = strings.TrimSpace(text)
		if text == "" {
			return ""
		}
		if parsed, err := time.Parse(time.RFC3339Nano, text); err == nil {
			return parsed.Add(duration).Format(time.RFC3339)
		}
		if number, err := strconv.ParseInt(text, 10, 64); err == nil {
			return formatUnixTimeWithOffset(number, duration)
		}
		return ""
	}
	if number, ok := int64Value(value); ok {
		return formatUnixTimeWithOffset(number, duration)
	}
	return ""
}

func formatUnixTimeWithOffset(value int64, duration time.Duration) string {
	if value <= 0 {
		return ""
	}
	var parsed time.Time
	if value > 100_000_000_000 {
		parsed = time.UnixMilli(value)
	} else {
		parsed = time.Unix(value, 0)
	}
	return parsed.Add(duration).Format(time.RFC3339)
}

func durationWindowLabel(seconds int64) string {
	if seconds <= 0 {
		return "当前周期"
	}
	switch {
	case seconds%604800 == 0:
		weeks := seconds / 604800
		if weeks == 1 {
			return "每周配额"
		}
		return fmt.Sprintf("%d 周配额", weeks)
	case seconds%86400 == 0:
		days := seconds / 86400
		if days == 1 {
			return "每日配额"
		}
		return fmt.Sprintf("%d 天配额", days)
	case seconds%3600 == 0:
		hours := seconds / 3600
		return fmt.Sprintf("%d 小时配额", hours)
	case seconds%60 == 0:
		return fmt.Sprintf("%d 分钟配额", seconds/60)
	default:
		return "当前周期"
	}
}

func localizedQuotaLabel(value string) string {
	label := strings.TrimSpace(value)
	if label == "" {
		return "配额"
	}
	lower := strings.ToLower(label)
	switch {
	case strings.Contains(lower, "weekly") || strings.Contains(lower, "week limit"):
		return "每周配额"
	case strings.Contains(lower, "monthly") || strings.Contains(lower, "month limit"):
		return "每月配额"
	case strings.Contains(lower, "daily") || strings.Contains(lower, "day limit"):
		return "每日配额"
	}
	if strings.HasSuffix(lower, "h limit") {
		return strings.TrimSuffix(lower, "h limit") + " 小时配额"
	}
	if strings.HasSuffix(lower, "d limit") {
		return strings.TrimSuffix(lower, "d limit") + " 天配额"
	}
	if strings.HasSuffix(lower, "m limit") {
		return strings.TrimSuffix(lower, "m limit") + " 分钟配额"
	}
	return label
}
