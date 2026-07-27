// Package ui renders the balance dashboard and settings HTML pages.
package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/examples/plugin/balance-query/go/internal/balance"
)

// ── Dashboard page ────────────────────────────────────────────────────────────

// RenderDashboard renders the main balance dashboard page.
func RenderDashboard(results []balance.Result, ttlSeconds int, fetchedAt time.Time) []byte {
	var b bytes.Buffer

	b.WriteString(`<!doctype html>
<html lang="zh">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Balance Query</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;background:#0f1117;color:#e2e8f0;padding:1.5rem;line-height:1.5}
h1{font-size:1.35rem;font-weight:700;margin-bottom:.2rem;color:#f8fafc}
.meta{font-size:.78rem;color:#64748b;margin-bottom:1.25rem}
.meta a{color:#60a5fa;text-decoration:none}.meta a:hover{text-decoration:underline}
.topbar{display:flex;align-items:center;gap:.75rem;flex-wrap:wrap;margin-bottom:1.25rem}
button{background:#3b82f6;color:#fff;border:none;border-radius:.45rem;padding:.4rem .9rem;font-size:.82rem;cursor:pointer;font-weight:600}
button:hover{background:#2563eb}
button.ghost{background:transparent;border:1px solid #374151;color:#94a3b8}
button.ghost:hover{background:#1e2230;color:#e2e8f0}
.ttl{display:flex;align-items:center;gap:.4rem;font-size:.78rem;color:#64748b}
.ttl input{width:56px;background:#1e2230;border:1px solid #374151;color:#e2e8f0;border-radius:.35rem;padding:.15rem .35rem;font-size:.78rem}
.grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(290px,1fr));gap:.9rem}
.card{background:#1e2230;border:1px solid #2d3748;border-radius:.7rem;padding:1.1rem}
.card.err{border-color:#ef4444;background:#1a1010}
.card.uncfg{border-color:#ca8a04;background:#1a1700;opacity:.75}
.card-head{display:flex;align-items:baseline;justify-content:space-between;margin-bottom:.6rem}
.card-name{font-weight:600;font-size:.9rem;color:#f1f5f9}
.card-id{font-size:.68rem;color:#4b5563;font-family:monospace;max-width:120px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.quota{font-size:1.05rem;font-weight:700;color:#34d399;margin-bottom:.45rem}
.quota.err{color:#f87171}.quota.warn{color:#fbbf24}
.bar-wrap{background:#0f1117;border-radius:9999px;height:5px;overflow:hidden;margin-bottom:.45rem}
.bar{height:5px;border-radius:9999px;background:linear-gradient(90deg,#34d399,#3b82f6)}
.bar.warn{background:linear-gradient(90deg,#f59e0b,#ef4444)}
.tags{display:flex;gap:.35rem;flex-wrap:wrap;margin-top:.4rem}
.tag{font-size:.7rem;background:#0f1117;border:1px solid #374151;border-radius:.3rem;padding:.1rem .4rem;color:#94a3b8}
.tag .k{color:#4b5563}
.ts{font-size:.7rem;color:#4b5563;margin-top:.4rem}
.empty{color:#4b5563;font-size:.88rem;padding:2rem 0;text-align:center}
</style>
</head>
<body>
<h1>💰 Balance Query</h1>
`)

	fetchStr := "从未"
	if !fetchedAt.IsZero() {
		fetchStr = fetchedAt.Format("2006-01-02 15:04:05")
	}
	fmt.Fprintf(&b, `<p class="meta">上次刷新: %s &nbsp;·&nbsp; TTL: %ds &nbsp;·&nbsp;
<a href="?action=settings">⚙ 设置</a></p>
`, html.EscapeString(fetchStr), ttlSeconds)

	b.WriteString(`<div class="topbar">
<button onclick="location.href='?action=refresh'">🔄 立即刷新</button>
<div class="ttl">
  <label>缓存TTL(秒):</label>
  <input id="ttlv" type="number" min="10" max="86400">
  <button class="ghost" onclick="saveTTL()">保存</button>
</div>
</div>
`)

	if len(results) == 0 {
		b.WriteString(`<p class="empty">暂无数据。<a href="?action=settings" style="color:#60a5fa">去设置页面</a>为每个账户指定 Provider。</p>`)
	} else {
		b.WriteString(`<div class="grid">`)
		for _, r := range results {
			renderCard(&b, r)
		}
		b.WriteString(`</div>`)
	}

	fmt.Fprintf(&b, `<script>
document.getElementById('ttlv').value = %d;
function saveTTL(){
  const v=document.getElementById('ttlv').value;
  fetch('?action=set_ttl&ttl='+encodeURIComponent(v),{method:'POST'}).then(()=>location.reload());
}
</script>
</body></html>`, ttlSeconds)

	return b.Bytes()
}

func renderCard(b *bytes.Buffer, r balance.Result) {
	cls := "card"
	if r.Error != "" {
		if strings.Contains(r.Error, "not configured") {
			cls = "card uncfg"
		} else {
			cls = "card err"
		}
	}
	fmt.Fprintf(b, `<div class="%s">`, cls)
	fmt.Fprintf(b, `<div class="card-head"><span class="card-name">%s</span>`,
		html.EscapeString(r.Provider))
	if r.AuthID != "" {
		fmt.Fprintf(b, `<span class="card-id" title="%s">%s</span>`,
			html.EscapeString(r.AuthID), html.EscapeString(truncRune(r.AuthID, 16)))
	}
	b.WriteString(`</div>`)

	if r.Error != "" {
		qCls := "quota err"
		if strings.Contains(r.Error, "not configured") {
			qCls = "quota warn"
		}
		fmt.Fprintf(b, `<div class="%s">%s</div>`, qCls, html.EscapeString(r.Error))
	} else {
		display := r.QuotaDisplay
		if display == "" && r.BalanceUSD != 0 {
			display = fmt.Sprintf("$%.4f", r.BalanceUSD)
		}
		if display != "" {
			fmt.Fprintf(b, `<div class="quota">%s</div>`, html.EscapeString(display))
		}
		if r.TokensTotal > 0 {
			pct := float64(r.TokensUsed) / float64(r.TokensTotal) * 100
			if pct > 100 {
				pct = 100
			}
			barCls := "bar"
			if pct > 80 {
				barCls = "bar warn"
			}
			fmt.Fprintf(b, `<div class="bar-wrap"><div class="%s" style="width:%.1f%%"></div></div>`, barCls, pct)
		}
		var tags []string
		if r.Plan != "" {
			tags = append(tags, tag("Plan", r.Plan))
		}
		if r.ResetAt != "" {
			tags = append(tags, tag("重置", r.ResetAt))
		}
		for k, v := range r.Extra {
			tags = append(tags, tag(k, v))
		}
		if len(tags) > 0 {
			b.WriteString(`<div class="tags">`)
			for _, t := range tags {
				b.WriteString(t)
			}
			b.WriteString(`</div>`)
		}
		if !r.FetchedAt.IsZero() {
			fmt.Fprintf(b, `<div class="ts">%s</div>`, r.FetchedAt.Format("15:04:05"))
		}
	}
	b.WriteString(`</div>`)
}

func tag(k, v string) string {
	return fmt.Sprintf(`<span class="tag"><span class="k">%s:</span> %s</span>`,
		html.EscapeString(k), html.EscapeString(v))
}

// ── Settings page ─────────────────────────────────────────────────────────────

// AuthEntry is a CPA auth list entry passed in from main.go.
type AuthEntry struct {
	AuthIndex string
	Name      string
	Provider  string
}

// RenderSettings renders the settings page where users map each auth to a provider.
func RenderSettings(auths []AuthEntry, cfg balance.PluginConfig, saveErr string) []byte {
	var b bytes.Buffer

	b.WriteString(`<!doctype html>
<html lang="zh">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Balance Query — 设置</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;background:#0f1117;color:#e2e8f0;padding:1.5rem;line-height:1.5}
h1{font-size:1.35rem;font-weight:700;margin-bottom:.2rem;color:#f8fafc}
.sub{font-size:.8rem;color:#64748b;margin-bottom:1.5rem}
.sub a{color:#60a5fa;text-decoration:none}
table{width:100%;border-collapse:collapse;font-size:.85rem}
th{text-align:left;padding:.5rem .75rem;border-bottom:2px solid #2d3748;color:#94a3b8;font-weight:600;font-size:.78rem;text-transform:uppercase;letter-spacing:.04em}
td{padding:.55rem .75rem;border-bottom:1px solid #1e2230;vertical-align:top}
tr:hover td{background:#111827}
.idx{font-family:monospace;font-size:.78rem;color:#4b5563}
.name{font-weight:500;color:#e2e8f0}
.prov{font-size:.78rem;color:#64748b}
select{background:#1e2230;border:1px solid #374151;color:#e2e8f0;border-radius:.4rem;padding:.3rem .5rem;font-size:.82rem;width:100%;max-width:280px}
select:focus{outline:2px solid #3b82f6;border-color:#3b82f6}
.url-input{display:none;margin-top:.4rem}
.url-input input{background:#1e2230;border:1px solid #374151;color:#e2e8f0;border-radius:.4rem;padding:.3rem .5rem;font-size:.8rem;width:100%;max-width:340px}
.url-input input::placeholder{color:#4b5563}
.actions{margin-top:1.25rem;display:flex;gap:.75rem;align-items:center;flex-wrap:wrap}
button{background:#3b82f6;color:#fff;border:none;border-radius:.45rem;padding:.45rem 1rem;font-size:.85rem;cursor:pointer;font-weight:600}
button:hover{background:#2563eb}
button.ghost{background:transparent;border:1px solid #374151;color:#94a3b8}
button.ghost:hover{background:#1e2230;color:#e2e8f0}
.err{color:#f87171;font-size:.82rem;margin-top:.75rem}
.ok{color:#34d399;font-size:.82rem;margin-top:.75rem}
</style>
</head>
<body>
<h1>⚙ Balance Query 设置</h1>
<p class="sub">为每个 CPA 账户选择对应的 Provider 类型。<a href="?action=dashboard">← 返回仪表盘</a></p>
`)

	if saveErr != "" {
		fmt.Fprintf(&b, `<p class="err">保存失败: %s</p>`, html.EscapeString(saveErr))
	}

	b.WriteString(`<form method="POST" action="?action=save_config">
<table>
<thead><tr>
<th>账户 ID</th>
<th>名称 / Provider</th>
<th>类型</th>
<th>Base URL (Sub2API / New API)</th>
</tr></thead>
<tbody>
`)

	providerOptions := buildProviderOptions()

	for _, a := range auths {
		mapping := cfg.Mappings[a.AuthIndex]
		selectedProvider := string(mapping.Provider)
		baseURL := mapping.BaseURL

		fmt.Fprintf(&b, `<tr>
<td class="idx">%s</td>
<td><div class="name">%s</div><div class="prov">%s</div></td>
<td>
<select name="p_%s" onchange="onProviderChange(this,'u_%s')">
<option value="">— 未配置 —</option>
%s
</select>
</td>
<td>
<div class="url-input" id="u_%s" %s>
<input name="url_%s" type="url" placeholder="https://your-instance.example.com"
  value="%s">
</div>
</td>
</tr>`,
			html.EscapeString(a.AuthIndex),
			html.EscapeString(a.Name),
			html.EscapeString(a.Provider),
			html.EscapeString(a.AuthIndex),
			html.EscapeString(a.AuthIndex),
			buildOptionHTML(providerOptions, selectedProvider),
			html.EscapeString(a.AuthIndex),
			urlInputStyle(balance.ProviderType(selectedProvider)),
			html.EscapeString(a.AuthIndex),
			html.EscapeString(baseURL),
		)
	}

	b.WriteString(`</tbody></table>
<div class="actions">
<button type="submit">💾 保存配置</button>
<button type="button" class="ghost" onclick="location.href='?action=dashboard'">取消</button>
</div>
</form>
<script>
const URL_PROVIDERS = ["sub2api","newapi"];
function onProviderChange(sel, urlDivId) {
  const div = document.getElementById(urlDivId);
  if (!div) return;
  div.style.display = URL_PROVIDERS.includes(sel.value) ? 'block' : 'none';
}
</script>
</body></html>`)

	return b.Bytes()
}

type providerOption struct {
	Value string
	Label string
}

func buildProviderOptions() []providerOption {
	all := balance.AllProviders()
	opts := make([]providerOption, 0, len(all))
	for _, p := range all {
		opts = append(opts, providerOption{
			Value: string(p),
			Label: balance.ProviderLabel[p],
		})
	}
	return opts
}

func buildOptionHTML(opts []providerOption, selected string) string {
	var sb strings.Builder
	for _, o := range opts {
		sel := ""
		if o.Value == selected {
			sel = ` selected`
		}
		fmt.Fprintf(&sb, `<option value="%s"%s>%s</option>`,
			html.EscapeString(o.Value), sel, html.EscapeString(o.Label))
	}
	return sb.String()
}

func urlInputStyle(p balance.ProviderType) string {
	if balance.NeedsBaseURL(p) {
		return `style="display:block"`
	}
	return ""
}

// ── CLI table ─────────────────────────────────────────────────────────────────

// RenderCLITable renders a plain-text ASCII table for CLI output.
func RenderCLITable(results []balance.Result) string {
	if len(results) == 0 {
		return "no balance data\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%-28s %-16s %-6s %s\n", "Provider", "Auth ID", "St", "Quota")
	b.WriteString(strings.Repeat("─", 86) + "\n")
	for _, r := range results {
		st := "✓"
		quota := r.QuotaDisplay
		if r.Error != "" {
			st = "✗"
			quota = r.Error
		}
		if quota == "" && r.BalanceUSD != 0 {
			quota = fmt.Sprintf("$%.4f", r.BalanceUSD)
		}
		fmt.Fprintf(&b, "%-28s %-16s %-6s %s\n",
			truncRune(r.Provider, 26),
			truncRune(r.AuthID, 14),
			st,
			truncRune(quota, 38),
		)
	}
	return b.String()
}

func truncRune(s string, max int) string {
	rr := []rune(s)
	if len(rr) <= max {
		return s
	}
	return string(rr[:max]) + "…"
}

// ── JSON helpers ──────────────────────────────────────────────────────────────

// MarshalIndent returns indented JSON or the error as a string.
func MarshalIndent(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}
