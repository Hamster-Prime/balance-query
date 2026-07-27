// Package ui renders the balance query management page.
package ui

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/Hamster-Prime/balance-query/internal/balance"
)

type providerDefinition struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// RenderDashboard renders a self-contained page for OpenAI-compatible provider
// balance mappings and queries. Secrets are loaded by the page at runtime and
// are never interpolated into the generated HTML.
func RenderDashboard(ttlSeconds int) []byte {
	definitions := make([]providerDefinition, 0, len(balance.AllProviders()))
	for _, providerType := range balance.AllProviders() {
		definitions = append(definitions, providerDefinition{
			Value: string(providerType),
			Label: balance.ProviderLabel[providerType],
		})
	}
	definitionsJSON, _ := json.Marshal(definitions)

	page := strings.NewReplacer(
		"__TTL_SECONDS__", strconv.Itoa(ttlSeconds),
		"__PROVIDER_DEFINITIONS__", string(definitionsJSON),
	).Replace(dashboardHTML)
	return []byte(page)
}

const dashboardHTML = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="color-scheme" content="light dark">
<title>余额与配额</title>
<script>
(function () {
  "use strict";
  var THEME_KEY = "cli-proxy-theme";
  var TOKENS = [
    "--bg-secondary","--bg-primary","--bg-tertiary","--bg-hover","--bg-quinary",
    "--bg-error-light","--floating-surface","--floating-shadow","--text-primary",
    "--text-secondary","--text-tertiary","--text-quaternary","--text-muted",
    "--border-color","--border-secondary","--border-primary","--border-hover",
    "--primary-color","--primary-hover","--primary-active","--primary-contrast",
    "--success-color","--quota-medium-color","--warning-color","--error-color",
    "--danger-color","--info-color","--warning-bg","--warning-border","--warning-text",
    "--success-badge-bg","--success-badge-text","--success-badge-border",
    "--failure-badge-bg","--failure-badge-text","--failure-badge-border",
    "--count-badge-bg","--count-badge-text","--shadow","--shadow-lg","--radius-md",
    "--primary-8","--primary-10","--primary-30","--amber-color","--amber-text",
    "--amber-10","--amber-30","--destructive-color","--destructive-10",
    "--destructive-30","--muted-bg","--muted-foreground","--accent-bg",
    "--glass-blur","--glass-backdrop-filter","--glass-filter","--glass-bg",
    "--glass-bg-secondary","--glass-border"
  ];
  var root = document.documentElement;
  var media = window.matchMedia ? window.matchMedia("(prefers-color-scheme: dark)") : null;

  function normalizeTheme(value) {
    if (value === "dark" || value === "white") return value;
    if (value === "auto") return media && media.matches ? "dark" : "white";
    return "light";
  }

  function applyTheme(value) {
    var theme = normalizeTheme(value);
    if (theme === "light") root.removeAttribute("data-theme");
    else root.setAttribute("data-theme", theme);
    root.style.colorScheme = theme === "dark" ? "dark" : "light";
  }

  function readStoredTheme() {
    try {
      var raw = localStorage.getItem(THEME_KEY);
      if (!raw) return "auto";
      var parsed = JSON.parse(raw);
      return (parsed && parsed.state && parsed.state.theme) || parsed.theme || "auto";
    } catch (_) {
      return "auto";
    }
  }

  function sameOriginParentRoot() {
    try {
      if (window.parent !== window && window.parent.location.origin === window.location.origin) {
        return window.parent.document.documentElement;
      }
    } catch (_) {}
    return null;
  }

  function copyParentTheme(parentRoot) {
    if (!parentRoot) return false;
    var parentTheme = parentRoot.getAttribute("data-theme");
    applyTheme(parentTheme === "dark" || parentTheme === "white" ? parentTheme : "light");
    try {
      var computed = window.parent.getComputedStyle(parentRoot);
      TOKENS.forEach(function (token) {
        var value = computed.getPropertyValue(token);
        if (value && value.trim()) root.style.setProperty(token, value.trim());
      });
    } catch (_) {}
    return true;
  }

  var parentRoot = sameOriginParentRoot();
  if (!copyParentTheme(parentRoot)) applyTheme(readStoredTheme());

  if (parentRoot && window.MutationObserver) {
    new MutationObserver(function () { copyParentTheme(parentRoot); }).observe(parentRoot, {
      attributes: true,
      attributeFilter: ["data-theme", "class", "style"]
    });
  }

  function onMediaChange() {
    if (!sameOriginParentRoot() && readStoredTheme() === "auto") applyTheme("auto");
  }
  if (media && media.addEventListener) media.addEventListener("change", onMediaChange);

  window.addEventListener("storage", function (event) {
    if (event.key === THEME_KEY && !sameOriginParentRoot()) applyTheme(readStoredTheme());
  });

  window.addEventListener("message", function (event) {
    if (event.origin !== window.location.origin || event.source !== window.parent) return;
    var data = event.data;
    if (!data || typeof data !== "object") return;
    var payload = data.detail || data.payload || data;
    var type = String(data.type || "").toLowerCase();
    var announcedTheme = payload.theme || payload.dataTheme || payload["data-theme"];
    if (announcedTheme && (!type || type.indexOf("theme") !== -1)) applyTheme(announcedTheme);
    var variables = payload.variables || payload.tokens || payload.cssVariables;
    if (variables && typeof variables === "object") {
      TOKENS.forEach(function (token) {
        if (typeof variables[token] === "string" && variables[token].trim()) {
          root.style.setProperty(token, variables[token].trim());
        }
      });
    }
  });
})();
</script>
<style>
:root {
  --bg-secondary:#faf9f5;
  --bg-primary:#f0eee8;
  --bg-tertiary:#e9e6df;
  --bg-hover:var(--bg-tertiary);
  --bg-quinary:#f6f4ee;
  --bg-error-light:rgba(198,87,70,.1);
  --floating-surface:#fffdf9;
  --floating-shadow:0 12px 26px rgba(0,0,0,.14);
  --text-primary:#2d2a26;
  --text-secondary:#6d6760;
  --text-tertiary:#a29c95;
  --text-quaternary:#c0bab3;
  --text-muted:var(--text-tertiary);
  --border-color:#e3e1db;
  --border-secondary:var(--border-color);
  --border-primary:#d5d2cb;
  --border-hover:#cecac4;
  --primary-color:#8b8680;
  --primary-hover:#7f7a74;
  --primary-active:#726d67;
  --primary-contrast:#fff;
  --success-color:#10b981;
  --quota-medium-color:#e0aa14;
  --warning-color:#c65746;
  --error-color:#c65746;
  --danger-color:var(--error-color);
  --info-color:var(--primary-color);
  --warning-bg:rgba(198,87,70,.12);
  --warning-border:rgba(198,87,70,.35);
  --warning-text:var(--warning-color);
  --success-badge-bg:#d1fae5;
  --success-badge-text:#065f46;
  --success-badge-border:#6ee7b7;
  --failure-badge-bg:rgba(198,87,70,.14);
  --failure-badge-text:#8a3a30;
  --failure-badge-border:rgba(198,87,70,.35);
  --count-badge-bg:rgba(139,134,128,.18);
  --count-badge-text:var(--primary-active);
  --shadow:0 1px 2px 0 rgb(0 0 0 / .08);
  --shadow-lg:0 10px 18px -3px rgb(0 0 0 / .1);
  --radius-md:8px;
  --primary-8:color-mix(in srgb,var(--primary-color) 8%,transparent);
  --primary-10:color-mix(in srgb,var(--primary-color) 10%,transparent);
  --primary-30:color-mix(in srgb,var(--primary-color) 30%,transparent);
  --amber-color:#d97706;
  --amber-text:#92400e;
  --amber-10:color-mix(in srgb,var(--amber-color) 10%,transparent);
  --amber-30:color-mix(in srgb,var(--amber-color) 30%,transparent);
  --destructive-color:var(--error-color);
  --destructive-10:color-mix(in srgb,var(--destructive-color) 10%,transparent);
  --destructive-30:color-mix(in srgb,var(--destructive-color) 30%,transparent);
  --muted-bg:var(--bg-tertiary);
  --muted-foreground:var(--text-secondary);
  --accent-bg:var(--bg-tertiary);
  --glass-blur:12px;
  --glass-backdrop-filter:blur(var(--glass-blur));
  --glass-filter:blur(var(--glass-blur));
  --glass-bg:color-mix(in srgb,var(--bg-primary) 82%,transparent);
  --glass-bg-secondary:color-mix(in srgb,var(--bg-secondary) 82%,transparent);
  --glass-border:color-mix(in srgb,var(--border-color) 60%,transparent);
}
[data-theme="white"] {
  --bg-secondary:#fff;
  --bg-primary:#fff;
  --bg-tertiary:#f6f6f6;
  --bg-hover:var(--bg-tertiary);
  --bg-quinary:#fff;
  --bg-error-light:rgba(198,87,70,.08);
  --floating-surface:#fff;
  --floating-shadow:0 12px 26px rgba(0,0,0,.12);
  --text-primary:#2d2a26;
  --text-secondary:#6d6760;
  --text-tertiary:#a29c95;
  --text-quaternary:#c0bab3;
  --text-muted:var(--text-tertiary);
  --border-color:#e5e5e5;
  --border-secondary:var(--border-color);
  --border-primary:#d9d9d9;
  --border-hover:#ccc;
  --primary-color:#8b8680;
  --primary-hover:#7f7a74;
  --primary-active:#726d67;
  --primary-contrast:#fff;
  --success-color:#10b981;
  --quota-medium-color:#e0aa14;
  --warning-color:#c65746;
  --error-color:#c65746;
  --danger-color:var(--error-color);
  --info-color:var(--primary-color);
  --warning-bg:rgba(198,87,70,.12);
  --warning-border:rgba(198,87,70,.35);
  --warning-text:var(--warning-color);
  --success-badge-bg:#d1fae5;
  --success-badge-text:#065f46;
  --success-badge-border:#6ee7b7;
  --failure-badge-bg:rgba(198,87,70,.14);
  --failure-badge-text:#8a3a30;
  --failure-badge-border:rgba(198,87,70,.35);
  --count-badge-bg:rgba(139,134,128,.18);
  --count-badge-text:var(--primary-active);
  --shadow:0 1px 2px 0 rgb(0 0 0 / .08);
  --shadow-lg:0 10px 18px -3px rgb(0 0 0 / .1);
  --radius-md:8px;
  --primary-8:color-mix(in srgb,var(--primary-color) 8%,transparent);
  --primary-10:color-mix(in srgb,var(--primary-color) 10%,transparent);
  --primary-30:color-mix(in srgb,var(--primary-color) 30%,transparent);
  --amber-color:#d97706;
  --amber-text:#92400e;
  --amber-10:color-mix(in srgb,var(--amber-color) 10%,transparent);
  --amber-30:color-mix(in srgb,var(--amber-color) 30%,transparent);
  --destructive-color:var(--error-color);
  --destructive-10:color-mix(in srgb,var(--destructive-color) 10%,transparent);
  --destructive-30:color-mix(in srgb,var(--destructive-color) 30%,transparent);
  --muted-bg:var(--bg-tertiary);
  --muted-foreground:var(--text-secondary);
  --accent-bg:var(--bg-tertiary);
}
[data-theme="dark"] {
  --bg-secondary:#151412;
  --bg-primary:#1d1b18;
  --bg-tertiary:#262320;
  --bg-hover:#2e2a26;
  --bg-quinary:#191714;
  --bg-error-light:rgba(198,87,70,.18);
  --floating-surface:#2a2723;
  --floating-shadow:0 14px 30px rgba(0,0,0,.4);
  --text-primary:#f6f4f1;
  --text-secondary:#c9c3bb;
  --text-tertiary:#9c958d;
  --text-quaternary:#6f6962;
  --text-muted:var(--text-tertiary);
  --border-color:#3a3530;
  --border-secondary:var(--border-color);
  --border-primary:#4a453f;
  --border-hover:#5a544d;
  --primary-color:#8b8680;
  --primary-hover:#9a948e;
  --primary-active:#a6a099;
  --primary-contrast:#fff;
  --success-color:#10b981;
  --quota-medium-color:#ffd862;
  --warning-color:#c65746;
  --error-color:#c65746;
  --danger-color:var(--error-color);
  --info-color:var(--primary-color);
  --warning-bg:rgba(198,87,70,.22);
  --warning-border:rgba(198,87,70,.45);
  --warning-text:#f1b0a6;
  --success-badge-bg:rgba(6,78,59,.3);
  --success-badge-text:#6ee7b7;
  --success-badge-border:#059669;
  --failure-badge-bg:rgba(198,87,70,.24);
  --failure-badge-text:#f1b0a6;
  --failure-badge-border:rgba(198,87,70,.5);
  --count-badge-bg:rgba(139,134,128,.28);
  --count-badge-text:var(--primary-active);
  --shadow:0 1px 3px 0 rgb(0 0 0 / .3);
  --shadow-lg:0 10px 15px -3px rgb(0 0 0 / .3);
  --radius-md:8px;
  --primary-8:color-mix(in srgb,var(--primary-color) 8%,transparent);
  --primary-10:color-mix(in srgb,var(--primary-color) 10%,transparent);
  --primary-30:color-mix(in srgb,var(--primary-color) 30%,transparent);
  --amber-color:#f59e0b;
  --amber-text:#fcd34d;
  --amber-10:color-mix(in srgb,var(--amber-color) 14%,transparent);
  --amber-30:color-mix(in srgb,var(--amber-color) 38%,transparent);
  --destructive-color:var(--error-color);
  --destructive-10:color-mix(in srgb,var(--destructive-color) 14%,transparent);
  --destructive-30:color-mix(in srgb,var(--destructive-color) 38%,transparent);
  --muted-bg:var(--bg-tertiary);
  --muted-foreground:var(--text-secondary);
  --accent-bg:var(--bg-tertiary);
  --glass-border:color-mix(in srgb,var(--border-color) 55%,transparent);
}
*{box-sizing:border-box;letter-spacing:0}
html,body{margin:0;min-width:280px;min-height:100%;background:var(--bg-secondary);color:var(--text-primary)}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI","Roboto","Oxygen","Ubuntu","Cantarell","Helvetica Neue",sans-serif;font-size:14px;line-height:1.5}
button,input,select{font:inherit;letter-spacing:0}
button{color:inherit}
[hidden]{display:none!important}
.icon{width:16px;height:16px;display:block;flex:0 0 auto;fill:none;stroke:currentColor;stroke-width:2;stroke-linecap:round;stroke-linejoin:round}
.app{width:100%;max-width:1220px;margin:0 auto;padding:24px clamp(16px,3vw,36px) 40px;animation:page-in 300ms ease-out both}
.masthead{display:flex;align-items:flex-start;justify-content:space-between;gap:20px;padding-bottom:18px;border-bottom:1px solid var(--border-color)}
.brand{display:flex;align-items:center;gap:12px;min-width:0}
.brand-mark{width:38px;height:38px;display:grid;place-items:center;flex:0 0 auto;border:1px solid var(--border-color);border-radius:8px;background:var(--bg-primary);color:var(--text-primary);box-shadow:var(--shadow)}
.brand-mark .icon{width:20px;height:20px}
h1{font-size:22px;line-height:1.25;font-weight:650;margin:0;color:var(--text-primary)}
.subtitle{margin:3px 0 0;color:var(--text-secondary);font-size:13px}
.head-state{display:flex;align-items:center;gap:8px;color:var(--text-secondary);font-size:13px;white-space:nowrap;padding-top:8px}
.state-dot{width:7px;height:7px;border-radius:50%;background:var(--success-color);box-shadow:0 0 0 3px color-mix(in srgb,var(--success-color) 14%,transparent)}
.state-dot.pending{background:var(--quota-medium-color);box-shadow:0 0 0 3px var(--amber-10)}
.workspace-nav{display:flex;align-items:center;justify-content:space-between;gap:16px;padding:18px 0 14px}
.segments{display:inline-grid;grid-template-columns:repeat(2,minmax(116px,1fr));padding:3px;border:1px solid var(--border-color);border-radius:8px;background:var(--bg-tertiary)}
.segment{height:34px;border:0;border-radius:6px;background:transparent;color:var(--text-secondary);padding:0 12px;display:inline-flex;align-items:center;justify-content:center;gap:7px;cursor:pointer;font-weight:600;font-size:13px;transition:background 150ms ease,color 150ms ease,box-shadow 150ms ease}
.segment:hover{color:var(--text-primary)}
.segment[aria-selected="true"]{background:var(--bg-primary);color:var(--text-primary);box-shadow:var(--shadow)}
.toolbar{display:flex;align-items:center;gap:8px;flex-wrap:wrap;justify-content:flex-end}
.btn{height:36px;border:1px solid transparent;border-radius:8px;padding:0 12px;display:inline-flex;align-items:center;justify-content:center;gap:7px;cursor:pointer;font-weight:600;font-size:13px;transition:background 150ms ease,border-color 150ms ease,color 150ms ease,transform 150ms ease,box-shadow 150ms ease}
.btn:hover:not(:disabled){transform:translateY(-1px)}
.btn:active:not(:disabled){transform:translateY(0)}
.btn:focus-visible,.segment:focus-visible,select:focus-visible,input:focus-visible{outline:2px solid var(--primary-color);outline-offset:2px}
.btn-primary{background:var(--primary-color);border-color:var(--primary-color);color:var(--primary-contrast)}
.btn-primary:hover:not(:disabled){background:var(--primary-hover);border-color:var(--primary-hover)}
.btn-secondary{background:var(--bg-primary);border-color:var(--border-color);color:var(--text-primary)}
.btn-secondary:hover:not(:disabled){background:var(--bg-hover);border-color:var(--border-hover)}
.btn-ghost{background:transparent;border-color:transparent;color:var(--text-secondary)}
.btn-ghost:hover:not(:disabled){background:var(--bg-tertiary);color:var(--text-primary)}
.btn:disabled{opacity:.55;cursor:not-allowed}
.spinner{width:15px;height:15px;border:2px solid currentColor;border-right-color:transparent;border-radius:50%;animation:spin .9s linear infinite}
.summary{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));border-top:1px solid var(--border-color);border-bottom:1px solid var(--border-color);margin-bottom:18px}
.summary-item{padding:14px 18px;min-width:0}
.summary-item+ .summary-item{border-left:1px solid var(--border-color)}
.summary-value{display:block;color:var(--text-primary);font-size:20px;font-weight:650;font-variant-numeric:tabular-nums;line-height:1.2}
.summary-label{display:block;color:var(--text-tertiary);font-size:12px;margin-top:4px}
.section-head{display:flex;align-items:flex-end;justify-content:space-between;gap:16px;margin:0 0 12px}
.section-title{font-size:15px;font-weight:650;margin:0;color:var(--text-primary)}
.section-meta{font-size:12px;color:var(--text-tertiary);margin:2px 0 0}
.result-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(280px,1fr));gap:10px}
.result-card{min-width:0;min-height:176px;padding:16px;border:1px solid var(--border-color);border-radius:8px;background:var(--bg-primary);box-shadow:var(--shadow);animation:item-in 300ms ease-out both;transition:border-color 150ms ease,box-shadow 150ms ease,transform 150ms ease}
.result-card:hover{border-color:var(--border-hover);box-shadow:var(--shadow-lg);transform:translateY(-1px)}
.result-card.error{border-color:var(--failure-badge-border)}
.result-head{display:flex;align-items:flex-start;justify-content:space-between;gap:12px;min-width:0}
.result-name{font-weight:650;color:var(--text-primary);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.result-url{font-family:"SFMono-Regular",Consolas,"Liberation Mono",monospace;color:var(--text-tertiary);font-size:11px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;margin-top:2px}
.badge{display:inline-flex;align-items:center;gap:5px;border-radius:9999px;border:1px solid var(--border-color);padding:3px 8px;font-size:11px;font-weight:600;white-space:nowrap}
.badge.success{color:var(--success-badge-text);background:var(--success-badge-bg);border-color:var(--success-badge-border)}
.badge.failure{color:var(--failure-badge-text);background:var(--failure-badge-bg);border-color:var(--failure-badge-border)}
.badge.muted{color:var(--text-secondary);background:var(--bg-tertiary)}
.quota-main{font-size:21px;line-height:1.25;font-weight:680;color:var(--text-primary);margin-top:18px;overflow-wrap:anywhere}
.quota-main.failure{font-size:14px;color:var(--warning-text);font-weight:600;line-height:1.45}
.error-detail{margin-top:6px;color:var(--text-secondary);font-size:12px;line-height:1.45;overflow-wrap:anywhere}
.progress-track{height:6px;border-radius:9999px;background:var(--bg-tertiary);overflow:hidden;margin-top:12px}
.progress-bar{height:100%;width:0;border-radius:inherit;background:var(--success-color);transition:width 300ms ease}
.progress-bar.medium{background:var(--quota-medium-color)}
.progress-bar.high{background:var(--error-color)}
.result-foot{display:flex;align-items:center;justify-content:space-between;gap:8px;margin-top:13px;color:var(--text-tertiary);font-size:11px}
.key-preview{font-family:"SFMono-Regular",Consolas,"Liberation Mono",monospace;color:var(--text-secondary)}
.detail-list{display:flex;gap:6px;flex-wrap:wrap;margin-top:10px}
.detail{display:inline-flex;gap:4px;border-radius:5px;background:var(--bg-tertiary);color:var(--text-secondary);padding:3px 6px;font-size:11px;max-width:100%;overflow-wrap:anywhere}
.detail-label{color:var(--text-tertiary)}
.notice{display:flex;align-items:flex-start;gap:10px;padding:12px 14px;border:1px solid var(--warning-border);border-radius:8px;background:var(--warning-bg);color:var(--warning-text);font-size:13px;margin-bottom:14px;animation:item-in 300ms ease-out both}
.notice .icon{margin-top:1px}
.empty-state{min-height:260px;border:1px dashed var(--border-color);border-radius:8px;display:flex;flex-direction:column;align-items:center;justify-content:center;text-align:center;padding:32px;color:var(--text-secondary)}
.empty-icon{width:42px;height:42px;display:grid;place-items:center;border-radius:8px;background:var(--bg-tertiary);color:var(--text-secondary);margin-bottom:12px}
.empty-icon .icon{width:20px;height:20px}
.empty-title{font-weight:650;color:var(--text-primary);margin:0 0 4px}
.empty-desc{font-size:13px;max-width:480px;margin:0 0 14px;color:var(--text-secondary)}
.settings-sheet{border:1px solid var(--border-color);border-radius:8px;background:var(--bg-primary);box-shadow:var(--shadow);overflow:hidden;animation:item-in 300ms ease-out both}
.settings-toolbar{display:flex;align-items:center;justify-content:space-between;gap:16px;padding:14px 16px;border-bottom:1px solid var(--border-color);background:var(--bg-quinary)}
.ttl-field{display:flex;align-items:center;gap:8px;color:var(--text-secondary);font-size:13px}
.ttl-field input{width:92px;height:34px;border:1px solid var(--border-color);border-radius:8px;background:var(--bg-primary);color:var(--text-primary);padding:0 9px;font-variant-numeric:tabular-nums}
.table-wrap{width:100%;overflow-x:auto}
table{width:100%;border-collapse:collapse;table-layout:fixed}
th{height:38px;padding:0 14px;text-align:left;color:var(--text-tertiary);font-size:11px;font-weight:650;background:var(--bg-tertiary);border-bottom:1px solid var(--border-color)}
td{padding:12px 14px;border-bottom:1px solid var(--border-color);vertical-align:middle}
tbody tr:last-child td{border-bottom:0}
tbody tr{transition:background 150ms ease}
tbody tr:hover{background:var(--bg-hover)}
.provider-cell{min-width:0}
.provider-title-line{display:flex;align-items:center;gap:7px;min-width:0}
.provider-title{font-weight:620;color:var(--text-primary);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.provider-disabled{color:var(--warning-text);font-size:11px;white-space:nowrap}
.provider-base{font-family:"SFMono-Regular",Consolas,"Liberation Mono",monospace;color:var(--text-tertiary);font-size:11px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;margin-top:3px}
.key-list{display:flex;align-items:center;gap:5px;flex-wrap:wrap}
.key-chip{font-family:"SFMono-Regular",Consolas,"Liberation Mono",monospace;border:1px solid var(--border-color);border-radius:5px;background:var(--bg-tertiary);color:var(--text-secondary);font-size:10px;padding:2px 5px}
select{width:100%;height:36px;border:1px solid var(--border-color);border-radius:8px;background:var(--bg-secondary);color:var(--text-primary);padding:0 30px 0 10px;transition:border-color 150ms ease,box-shadow 150ms ease}
select:hover{border-color:var(--border-hover)}
.save-state{min-height:18px;color:var(--text-tertiary);font-size:12px}
.connection-shell{min-height:calc(100vh - 48px);display:grid;place-items:center;padding:24px}
.connection-card{width:min(100%,430px);border:1px solid var(--border-color);border-radius:8px;background:var(--bg-primary);box-shadow:var(--shadow-lg);padding:22px;animation:item-in 300ms ease-out both}
.connection-head{display:flex;align-items:flex-start;gap:12px;margin-bottom:20px}
.connection-title{margin:0;font-size:18px;font-weight:650}
.connection-desc{margin:3px 0 0;color:var(--text-secondary);font-size:13px}
.field{display:flex;flex-direction:column;gap:6px;margin-top:13px}
.field label{font-weight:600;font-size:13px;color:var(--text-primary)}
.field input{width:100%;height:38px;border:1px solid var(--border-color);border-radius:8px;background:var(--bg-secondary);color:var(--text-primary);padding:0 11px;transition:border-color 150ms ease,box-shadow 150ms ease}
.field input::placeholder{color:var(--text-tertiary)}
.field input:focus,.ttl-field input:focus,select:focus{border-color:var(--primary-color);box-shadow:0 0 0 3px var(--primary-10);outline:0}
.connection-actions{display:flex;justify-content:flex-end;margin-top:18px}
.form-error{min-height:20px;margin:10px 0 0;color:var(--warning-text);font-size:12px}
.skeleton-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(280px,1fr));gap:10px}
.skeleton-card{height:176px;border:1px solid var(--border-color);border-radius:8px;background:var(--bg-primary);padding:16px;overflow:hidden}
.skeleton{position:relative;overflow:hidden;background:var(--bg-tertiary);border-radius:5px}
.skeleton::after{content:"";position:absolute;inset:0;transform:translateX(-100%);background:linear-gradient(90deg,transparent,color-mix(in srgb,var(--text-primary) 7%,transparent),transparent);animation:skeleton 1.5s ease-in-out infinite}
.skeleton-line{height:12px;margin-bottom:10px}.skeleton-line.short{width:42%}.skeleton-line.medium{width:68%}.skeleton-line.value{width:54%;height:24px;margin-top:25px}
.toast-region{position:fixed;z-index:20;right:18px;bottom:18px;display:flex;flex-direction:column;gap:8px;pointer-events:none}
.toast{max-width:min(390px,calc(100vw - 36px));display:flex;align-items:flex-start;gap:8px;padding:10px 12px;border:1px solid var(--border-color);border-radius:8px;background:var(--floating-surface);box-shadow:var(--floating-shadow);color:var(--text-primary);font-size:13px;animation:toast-in 300ms ease-out both}
.toast.error{border-color:var(--failure-badge-border)}
@keyframes spin{to{transform:rotate(360deg)}}
@keyframes skeleton{100%{transform:translateX(100%)}}
@keyframes page-in{from{opacity:0}to{opacity:1}}
@keyframes item-in{from{opacity:0;transform:translateY(6px)}to{opacity:1;transform:translateY(0)}}
@keyframes toast-in{from{opacity:0;transform:translateY(8px)}to{opacity:1;transform:translateY(0)}}
@media (max-width:760px){
  .app{padding:16px 14px 28px}.masthead{align-items:center}.subtitle{max-width:230px}.head-state{display:none}
  .workspace-nav{align-items:stretch;flex-direction:column}.segments{width:100%}.toolbar{justify-content:stretch}.toolbar .btn{flex:1}
  .summary{grid-template-columns:repeat(2,minmax(0,1fr))}.summary-item:nth-child(3){border-left:0;border-top:1px solid var(--border-color)}.summary-item:nth-child(4){border-top:1px solid var(--border-color)}
  .result-grid,.skeleton-grid{grid-template-columns:1fr}
  .settings-toolbar{align-items:flex-start;flex-direction:column}.ttl-field{width:100%;justify-content:space-between}
  table,thead,tbody,tr,th,td{display:block}thead{display:none}table{table-layout:auto}tbody tr{padding:13px 14px;border-bottom:1px solid var(--border-color)}tbody tr:last-child{border-bottom:0}td{padding:0;border:0}td+td{margin-top:10px}.provider-base{white-space:normal;overflow-wrap:anywhere}.query-cell::before{content:"余额查询类型";display:block;color:var(--text-tertiary);font-size:11px;margin-bottom:5px}
}
@media (max-width:420px){.btn-label.optional{display:none}.summary-item{padding:12px}.result-card{padding:14px}.connection-shell{padding:14px}.connection-card{padding:18px}}
@media (prefers-reduced-motion:reduce){*,*::before,*::after{scroll-behavior:auto!important;animation-duration:.001ms!important;animation-iteration-count:1!important;transition-duration:.001ms!important}.btn:hover:not(:disabled),.result-card:hover{transform:none}}
@media (max-width:768px),(prefers-reduced-motion:reduce),(prefers-reduced-transparency:reduce){:root{--glass-backdrop-filter:none;--glass-filter:none;--glass-bg:var(--bg-primary);--glass-bg-secondary:var(--bg-secondary);--glass-border:var(--border-color)}}
</style>
</head>
<body>
<svg aria-hidden="true" width="0" height="0" style="position:absolute;overflow:hidden">
  <symbol id="i-wallet" viewBox="0 0 24 24"><rect width="20" height="14" x="2" y="5" rx="2"/><path d="M16 13h4"/><path d="M16 9h4"/><path d="M6 9h4"/><path d="M6 13h3"/></symbol>
  <symbol id="i-dashboard" viewBox="0 0 24 24"><rect width="7" height="9" x="3" y="3" rx="1"/><rect width="7" height="5" x="14" y="3" rx="1"/><rect width="7" height="9" x="14" y="12" rx="1"/><rect width="7" height="5" x="3" y="16" rx="1"/></symbol>
  <symbol id="i-sliders" viewBox="0 0 24 24"><path d="M4 21v-7"/><path d="M4 10V3"/><path d="M12 21v-9"/><path d="M12 8V3"/><path d="M20 21v-5"/><path d="M20 12V3"/><path d="M1 14h6"/><path d="M9 8h6"/><path d="M17 16h6"/></symbol>
  <symbol id="i-refresh" viewBox="0 0 24 24"><path d="M20 6v6h-6"/><path d="M20 12a8 8 0 1 0-2.34 5.66L20 15"/></symbol>
  <symbol id="i-save" viewBox="0 0 24 24"><path d="M15.2 3H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V8.8z"/><path d="M17 21v-8H7v8"/><path d="M7 3v5h8"/></symbol>
  <symbol id="i-plug" viewBox="0 0 24 24"><path d="m8 12 4 4 6-6-4-4z"/><path d="m14 6 3-3"/><path d="m18 10 3-3"/><path d="m6 14-3 3a2.8 2.8 0 0 0 4 4l3-3"/></symbol>
  <symbol id="i-alert" viewBox="0 0 24 24"><path d="M21.73 18 13.73 4a2 2 0 0 0-3.46 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3"/><path d="M12 9v4"/><path d="M12 17h.01"/></symbol>
  <symbol id="i-check" viewBox="0 0 24 24"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><path d="m9 11 3 3L22 4"/></symbol>
  <symbol id="i-key" viewBox="0 0 24 24"><circle cx="7.5" cy="15.5" r="5.5"/><path d="m21 2-9.6 9.6"/><path d="m15.5 7.5 3 3L22 7l-3-3"/></symbol>
  <symbol id="i-server" viewBox="0 0 24 24"><rect width="20" height="8" x="2" y="2" rx="2"/><rect width="20" height="8" x="2" y="14" rx="2"/><path d="M6 6h.01"/><path d="M6 18h.01"/></symbol>
  <symbol id="i-arrow" viewBox="0 0 24 24"><path d="M5 12h14"/><path d="m13 6 6 6-6 6"/></symbol>
</svg>

<main id="app" class="app" hidden>
  <header class="masthead">
    <div class="brand">
      <div class="brand-mark"><svg class="icon" aria-hidden="true"><use href="#i-wallet"></use></svg></div>
      <div>
        <h1>余额与配额</h1>
        <p class="subtitle">查询 OpenAI 兼容提供商的账户余额与套餐配额</p>
      </div>
    </div>
    <div class="head-state" aria-live="polite"><span id="connection-dot" class="state-dot pending"></span><span id="connection-state">正在连接 CPA</span></div>
  </header>

  <div class="workspace-nav">
    <div class="segments" role="tablist" aria-label="页面视图">
      <button id="tab-overview" class="segment" type="button" role="tab" aria-selected="true" aria-controls="view-overview" data-view="overview"><svg class="icon" aria-hidden="true"><use href="#i-dashboard"></use></svg>概览</button>
      <button id="tab-settings" class="segment" type="button" role="tab" aria-selected="false" aria-controls="view-settings" data-view="settings"><svg class="icon" aria-hidden="true"><use href="#i-sliders"></use></svg>查询设置</button>
    </div>
    <div class="toolbar">
      <button id="reconnect-button" class="btn btn-ghost" type="button" aria-label="重新连接 CPA" title="重新连接 CPA"><svg class="icon" aria-hidden="true"><use href="#i-plug"></use></svg><span class="btn-label optional">重新连接</span></button>
      <button id="refresh-button" class="btn btn-secondary" type="button"><svg class="icon refresh-icon" aria-hidden="true"><use href="#i-refresh"></use></svg><span class="btn-label">刷新余额</span></button>
      <button id="save-button" class="btn btn-primary" type="button" hidden><svg class="icon" aria-hidden="true"><use href="#i-save"></use></svg><span class="btn-label">保存设置</span></button>
    </div>
  </div>

  <section id="view-overview" role="tabpanel" aria-labelledby="tab-overview">
    <div class="summary" aria-label="余额查询概览">
      <div class="summary-item"><span id="stat-providers" class="summary-value">0</span><span class="summary-label">兼容提供商</span></div>
      <div class="summary-item"><span id="stat-configured" class="summary-value">0</span><span class="summary-label">已配置查询</span></div>
      <div class="summary-item"><span id="stat-keys" class="summary-value">0</span><span class="summary-label">可查询密钥</span></div>
      <div class="summary-item"><span id="stat-success" class="summary-value">0</span><span class="summary-label">本次查询成功</span></div>
    </div>
    <div id="overview-notice"></div>
    <div class="section-head">
      <div><h2 class="section-title">账户余额</h2><p id="query-meta" class="section-meta">正在读取提供商配置</p></div>
    </div>
    <div id="results" aria-live="polite" aria-busy="true"></div>
  </section>

  <section id="view-settings" role="tabpanel" aria-labelledby="tab-settings" hidden>
    <div class="section-head">
      <div><h2 class="section-title">查询映射</h2><p class="section-meta">为每个 OpenAI 兼容提供商手动指定余额查询类型</p></div>
      <div id="save-state" class="save-state" aria-live="polite"></div>
    </div>
    <div class="settings-sheet">
      <div class="settings-toolbar">
        <div><strong>缓存策略</strong><div class="section-meta">设为 0 可关闭缓存，其他值为 10 至 86400 秒</div></div>
        <label class="ttl-field" for="ttl-input"><span>缓存时长</span><input id="ttl-input" type="number" min="0" max="86400" step="1" inputmode="numeric" aria-describedby="ttl-unit"><span id="ttl-unit">秒</span></label>
      </div>
      <div class="table-wrap">
        <table>
          <colgroup><col style="width:38%"><col style="width:28%"><col style="width:34%"></colgroup>
          <thead><tr><th scope="col">OpenAI 兼容提供商</th><th scope="col">接口密钥</th><th scope="col">余额查询类型</th></tr></thead>
          <tbody id="settings-body"></tbody>
        </table>
      </div>
      <div id="settings-empty" hidden></div>
    </div>
  </section>
</main>

<section id="connection-view" class="connection-shell" hidden>
  <form id="connection-form" class="connection-card" novalidate>
    <div class="connection-head">
      <div class="brand-mark"><svg class="icon" aria-hidden="true"><use href="#i-plug"></use></svg></div>
      <div><h1 class="connection-title">连接 CPA</h1><p class="connection-desc">未读取到管理凭据，请手动连接当前 CPA 实例。</p></div>
    </div>
    <div class="field"><label for="api-base-input">CPA 地址</label><input id="api-base-input" type="url" inputmode="url" autocomplete="url" placeholder="http://127.0.0.1:8317" required></div>
    <div class="field"><label for="management-key-input">管理密钥</label><input id="management-key-input" type="password" autocomplete="current-password" placeholder="输入管理密钥" required></div>
    <p id="connection-error" class="form-error" role="alert"></p>
    <div class="connection-actions"><button id="connect-button" class="btn btn-primary" type="submit"><svg class="icon" aria-hidden="true"><use href="#i-arrow"></use></svg><span>连接</span></button></div>
  </form>
</section>

<div id="toast-region" class="toast-region" aria-live="polite" aria-atomic="true"></div>

<script>
(function () {
  "use strict";
  var PROVIDER_DEFINITIONS = __PROVIDER_DEFINITIONS__;
  var INITIAL_TTL = __TTL_SECONDS__;
  var MANAGEMENT_PREFIX = "/v0/management";
  var AUTH_STORAGE_KEY = "cli-proxy-auth";
  var ENC_PREFIX = "enc::v1::";
  var SECRET_SALT = "cli-proxy-api-webui::secure-storage";
  var REQUEST_TIMEOUT = 30000;
  var state = {
    credentials: { apiBase: "", managementKey: "" },
    providers: [],
    globalProxyUrl: "",
    config: { cache_ttl_seconds: INITIAL_TTL, provider_mappings: {} },
    draftMappings: {},
    results: [],
    view: "overview",
    querying: false,
    saving: false,
    dirty: false
  };
  var providerLabels = Object.create(null);
  PROVIDER_DEFINITIONS.forEach(function (item) { providerLabels[item.value] = item.label; });

  function byID(id) { return document.getElementById(id); }
  function setText(node, value) { if (node) node.textContent = value == null ? "" : String(value); }
  function icon(name) {
    var svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    svg.setAttribute("class", "icon");
    svg.setAttribute("aria-hidden", "true");
    var use = document.createElementNS("http://www.w3.org/2000/svg", "use");
    use.setAttribute("href", "#i-" + name);
    svg.appendChild(use);
    return svg;
  }
  function element(tag, className, text) {
    var node = document.createElement(tag);
    if (className) node.className = className;
    if (text != null) node.textContent = String(text);
    return node;
  }

  function deobfuscate(raw) {
    if (!raw || raw.indexOf(ENC_PREFIX) !== 0) return raw;
    try {
      var binary = atob(raw.slice(ENC_PREFIX.length));
      var encrypted = new Uint8Array(binary.length);
      for (var i = 0; i < binary.length; i++) encrypted[i] = binary.charCodeAt(i);
      var key = new TextEncoder().encode(SECRET_SALT + "|" + window.location.host + "|" + navigator.userAgent);
      var clear = new Uint8Array(encrypted.length);
      for (var j = 0; j < encrypted.length; j++) clear[j] = encrypted[j] ^ key[j % key.length];
      return new TextDecoder().decode(clear);
    } catch (_) {
      return "";
    }
  }

  function parseStoredValue(raw) {
    var value = deobfuscate(raw);
    if (!value) return null;
    try {
      var parsed = JSON.parse(value);
      if (typeof parsed === "string") {
        try { parsed = JSON.parse(parsed); } catch (_) {}
      }
      return parsed;
    } catch (_) {
      return null;
    }
  }

  function sameOriginStorage() {
    try {
      if (window.parent !== window && window.parent.location.origin === window.location.origin) return window.parent.localStorage;
    } catch (_) {}
    try { return window.localStorage; } catch (_) { return null; }
  }

  function restoreCredentials() {
    var storage = sameOriginStorage();
    if (!storage) return null;
    try {
      var parsed = parseStoredValue(storage.getItem(AUTH_STORAGE_KEY));
      var storedState = parsed && parsed.state ? parsed.state : parsed;
      if (!storedState || typeof storedState !== "object") return null;
      var base = normalizeApiBase(storedState.apiBase || storedState.apiUrl || window.location.origin);
      var key = typeof storedState.managementKey === "string" ? storedState.managementKey.trim() : "";
      if (!base || !key) return null;
      return { apiBase: base, managementKey: key };
    } catch (_) {
      return null;
    }
  }

  function normalizeApiBase(input) {
    var value = String(input || "").trim().replace(/\/?v0\/management\/?$/i, "").replace(/\/+$/, "");
    if (!value) return "";
    if (!/^https?:\/\//i.test(value)) value = "http://" + value;
    try {
      var parsed = new URL(value);
      if ((parsed.protocol !== "http:" && parsed.protocol !== "https:") || parsed.username || parsed.password) return "";
      return parsed.href.replace(/\/+$/, "");
    } catch (_) {
      return "";
    }
  }

  function endpoint(path) { return state.credentials.apiBase + MANAGEMENT_PREFIX + path; }

  function statusMessage(status) {
    if (status === 400) return "请求参数不正确";
    if (status === 401 || status === 403) return "管理密钥无效或权限不足";
    if (status === 404) return "当前 CPA 版本不支持此接口";
    if (status === 409) return "配置已被其他操作更新，请刷新后重试";
    if (status >= 500) return "CPA 服务暂时不可用";
    return "请求未成功";
  }

  function apiFetch(path, options, timeout) {
    var controller = new AbortController();
    var timer = window.setTimeout(function () { controller.abort(); }, timeout || REQUEST_TIMEOUT);
    var init = Object.assign({}, options || {}, {
      cache: "no-store",
      credentials: "omit",
      signal: controller.signal,
      headers: Object.assign({
        "Accept": "application/json",
        "Authorization": "Bearer " + state.credentials.managementKey
      }, options && options.headers ? options.headers : {})
    });
    return fetch(endpoint(path), init).then(function (response) {
      return response.text().then(function (body) {
        var data = null;
        if (body) { try { data = JSON.parse(body); } catch (_) {} }
        if (!response.ok) {
          var error = new Error(statusMessage(response.status));
          error.status = response.status;
          throw error;
        }
        return data || {};
      });
    }).catch(function (error) {
      if (error && error.name === "AbortError") throw new Error("请求超时，请稍后重试");
      throw error;
    }).finally(function () { window.clearTimeout(timer); });
  }

  function normalizeBaseForKey(value) { return String(value || "").trim().replace(/\/+$/, ""); }
  function mappingKey(provider) {
    return encodeURIComponent(String(provider.name || "").trim()) + "|" + encodeURIComponent(normalizeBaseForKey(provider.baseUrl));
  }
  function normalizeProvider(raw, index) {
    var entries = Array.isArray(raw && raw["api-key-entries"]) ? raw["api-key-entries"] : [];
    var keys = entries.reduce(function (list, entry) {
      var apiKey = entry && typeof entry["api-key"] === "string" ? entry["api-key"] : "";
      if (!apiKey.trim()) return list;
      list.push({
        apiKey: apiKey,
        authIndex: String(entry["auth-index"] || ""),
        proxyUrl: String(entry["proxy-url"] || "")
      });
      return list;
    }, []);
    var provider = {
      name: String(raw && raw.name || ("未命名提供商 " + (index + 1))),
      baseUrl: String(raw && raw["base-url"] || ""),
      disabled: Boolean(raw && raw.disabled),
      keys: keys,
      index: index
    };
    provider.mappingKey = mappingKey(provider);
    return provider;
  }

  function normalizeConfig(raw) {
    var mappings = raw && raw.provider_mappings && typeof raw.provider_mappings === "object" ? raw.provider_mappings : {};
    var ttl = Number(raw && raw.cache_ttl_seconds);
    if (!Number.isInteger(ttl)) ttl = INITIAL_TTL;
    return { cache_ttl_seconds: ttl, provider_mappings: Object.assign({}, mappings) };
  }

  function showConnection(message) {
    byID("app").hidden = true;
    byID("connection-view").hidden = false;
    var defaultBase = state.credentials.apiBase || normalizeApiBase(window.location.origin);
    byID("api-base-input").value = defaultBase;
    byID("management-key-input").value = "";
    setText(byID("connection-error"), message || "");
    window.setTimeout(function () { byID(message ? "management-key-input" : "api-base-input").focus(); }, 0);
  }

  function showApp() {
    byID("connection-view").hidden = true;
    byID("app").hidden = false;
    byID("connection-dot").classList.remove("pending");
    setText(byID("connection-state"), "已连接 CPA");
  }

  function setButtonBusy(button, busy, label) {
    if (!button) return;
    button.disabled = busy;
    var labelNode = button.querySelector(".btn-label") || button.querySelector("span");
    if (labelNode && label) labelNode.textContent = label;
    var oldSpinner = button.querySelector(".spinner");
    if (busy && !oldSpinner) button.insertBefore(element("span", "spinner"), button.firstChild);
    if (!busy && oldSpinner) oldSpinner.remove();
  }

  function toast(message, isError) {
    var region = byID("toast-region");
    var item = element("div", "toast" + (isError ? " error" : ""));
    item.appendChild(icon(isError ? "alert" : "check"));
    item.appendChild(element("span", "", message));
    region.appendChild(item);
    window.setTimeout(function () {
      item.style.opacity = "0";
      item.style.transform = "translateY(6px)";
      window.setTimeout(function () { item.remove(); }, 300);
    }, 3000);
  }

  function setView(view) {
    state.view = view === "settings" ? "settings" : "overview";
    var overview = state.view === "overview";
    byID("view-overview").hidden = !overview;
    byID("view-settings").hidden = overview;
    byID("tab-overview").setAttribute("aria-selected", String(overview));
    byID("tab-settings").setAttribute("aria-selected", String(!overview));
    byID("refresh-button").hidden = !overview;
    byID("save-button").hidden = overview;
    if (!overview) renderSettings();
  }

  function showSkeletons() {
    var target = byID("results");
    target.setAttribute("aria-busy", "true");
    target.textContent = "";
    var grid = element("div", "skeleton-grid");
    for (var i = 0; i < 3; i++) {
      var card = element("div", "skeleton-card");
      card.appendChild(element("div", "skeleton skeleton-line medium"));
      card.appendChild(element("div", "skeleton skeleton-line short"));
      card.appendChild(element("div", "skeleton skeleton-line value"));
      card.appendChild(element("div", "skeleton skeleton-line medium"));
      grid.appendChild(card);
    }
    target.appendChild(grid);
  }

  function emptyState(target, title, description, actionLabel, action) {
    target.textContent = "";
    var box = element("div", "empty-state");
    var iconBox = element("div", "empty-icon");
    iconBox.appendChild(icon("server"));
    box.appendChild(iconBox);
    box.appendChild(element("p", "empty-title", title));
    box.appendChild(element("p", "empty-desc", description));
    if (actionLabel && action) {
      var button = element("button", "btn btn-secondary");
      button.type = "button";
      button.appendChild(icon("sliders"));
      button.appendChild(element("span", "", actionLabel));
      button.addEventListener("click", action);
      box.appendChild(button);
    }
    target.appendChild(box);
  }

  function maskKey(value) {
    var key = String(value || "").trim();
    if (!key) return "未提供";
    if (key.length <= 6) return "••••" + key.slice(-2);
    return key.slice(0, 3) + "••••••" + key.slice(-4);
  }

  function tinyHash(value) {
    var hash = 2166136261;
    for (var i = 0; i < value.length; i++) {
      hash ^= value.charCodeAt(i);
      hash = Math.imul(hash, 16777619);
    }
    return (hash >>> 0).toString(36);
  }

  function buildAccounts() {
    var accounts = [];
    state.providers.forEach(function (provider) {
      if (provider.disabled) return;
      var queryType = state.config.provider_mappings[provider.mappingKey];
      if (!providerLabels[queryType]) return;
      provider.keys.forEach(function (keyEntry, index) {
        accounts.push({
          id: keyEntry.authIndex || ("compat-" + tinyHash(provider.mappingKey) + "-" + (index + 1)),
          provider_key: provider.mappingKey,
          account_name: provider.name + " · 密钥 " + (index + 1),
          base_url: provider.baseUrl,
          api_key: keyEntry.apiKey,
          proxy_url: String(keyEntry.proxyUrl || "").trim() || state.globalProxyUrl,
          query_type: queryType
        });
      });
    });
    return accounts;
  }

  function configuredProviderCount() {
    return state.providers.filter(function (provider) {
      return !provider.disabled && Boolean(providerLabels[state.config.provider_mappings[provider.mappingKey]]);
    }).length;
  }

  function updateSummary() {
    var keyCount = state.providers.reduce(function (sum, provider) { return sum + (provider.disabled ? 0 : provider.keys.length); }, 0);
    var successCount = state.results.filter(function (result) { return !result.error; }).length;
    setText(byID("stat-providers"), state.providers.length);
    setText(byID("stat-configured"), configuredProviderCount());
    setText(byID("stat-keys"), keyCount);
    setText(byID("stat-success"), successCount);
  }

  function formatTime(value) {
    if (!value) return "";
    var date = new Date(value);
    if (Number.isNaN(date.getTime())) return "";
    return new Intl.DateTimeFormat("zh-CN", { hour:"2-digit", minute:"2-digit", second:"2-digit" }).format(date);
  }

  function formatBalance(result) {
    if (result.quota_display) return redactSecrets(result.quota_display);
    if (Object.prototype.hasOwnProperty.call(result, "balance_usd")) {
      var amount = Number(result.balance_usd);
      if (Number.isFinite(amount)) return "$" + amount.toFixed(amount >= 100 ? 2 : 4);
    }
    if (Number(result.tokens_total) > 0) {
      return Number(result.tokens_remaining || 0).toLocaleString("zh-CN") + " 可用令牌";
    }
    return "已获取账户信息";
  }

  function detailLabel(key) {
    var labels = {
      plan:"套餐",reset:"重置时间",currency:"币种",remaining:"剩余",used:"已使用",
      total:"总量",limit:"额度",requests:"请求次数",window:"统计周期",expires:"到期时间"
    };
    return labels[String(key || "").toLowerCase()] || "详情";
  }

  function redactSecrets(value) {
    var text = String(value == null ? "" : value);
    state.providers.forEach(function (provider) {
      provider.keys.forEach(function (entry) {
        if (entry.apiKey && text.indexOf(entry.apiKey) !== -1) {
          text = text.split(entry.apiKey).join(maskKey(entry.apiKey));
        }
      });
    });
    return text;
  }

  function resultCard(result, index) {
    var failed = Boolean(result.error);
    var card = element("article", "result-card" + (failed ? " error" : ""));
    card.style.animationDelay = Math.min(index * 35, 210) + "ms";
    var head = element("div", "result-head");
    var identity = element("div", "provider-cell");
    identity.appendChild(element("div", "result-name", result.account_name || result.provider || "余额账户"));
    identity.appendChild(element("div", "result-url", result.base_url || ""));
    head.appendChild(identity);
    var badge = element("span", "badge " + (failed ? "failure" : "success"));
    badge.appendChild(icon(failed ? "alert" : "check"));
    badge.appendChild(element("span", "", failed ? "查询失败" : "查询成功"));
    head.appendChild(badge);
    card.appendChild(head);

    if (failed) {
      card.appendChild(element("div", "quota-main failure", "查询失败，请检查密钥、接口地址或账户状态"));
      card.appendChild(element("div", "error-detail", redactSecrets(result.error)));
    } else {
      card.appendChild(element("div", "quota-main", formatBalance(result)));
      var total = Number(result.tokens_total || 0);
      var used = Number(result.tokens_used || 0);
      if (total > 0) {
        var percent = Math.max(0, Math.min(100, used / total * 100));
        var track = element("div", "progress-track");
        track.setAttribute("role", "progressbar");
        track.setAttribute("aria-label", "令牌使用进度");
        track.setAttribute("aria-valuemin", "0");
        track.setAttribute("aria-valuemax", "100");
        track.setAttribute("aria-valuenow", String(Math.round(percent)));
        var bar = element("div", "progress-bar" + (percent >= 85 ? " high" : percent >= 60 ? " medium" : ""));
        track.appendChild(bar);
        card.appendChild(track);
        window.requestAnimationFrame(function () { bar.style.width = percent.toFixed(1) + "%"; });
      }
      var details = element("div", "detail-list");
      if (result.plan) {
        var plan = element("span", "detail");
        plan.appendChild(element("span", "detail-label", "套餐"));
        plan.appendChild(element("span", "", redactSecrets(result.plan)));
        details.appendChild(plan);
      }
      if (result.reset_at) {
        var reset = element("span", "detail");
        reset.appendChild(element("span", "detail-label", "重置"));
        reset.appendChild(element("span", "", redactSecrets(result.reset_at)));
        details.appendChild(reset);
      }
      if (result.extra && typeof result.extra === "object") {
        Object.keys(result.extra).slice(0, 3).forEach(function (key) {
          var detail = element("span", "detail");
          detail.appendChild(element("span", "detail-label", detailLabel(key)));
          detail.appendChild(element("span", "", redactSecrets(result.extra[key])));
          details.appendChild(detail);
        });
      }
      if (details.childNodes.length) card.appendChild(details);
    }

    var foot = element("div", "result-foot");
    foot.appendChild(element("span", "key-preview", result.key_preview || "密钥已隐藏"));
    foot.appendChild(element("span", "", formatTime(result.fetched_at)));
    card.appendChild(foot);
    return card;
  }

  function renderNotice() {
    var target = byID("overview-notice");
    target.textContent = "";
    var unconfigured = state.providers.filter(function (provider) {
      return !provider.disabled && !providerLabels[state.config.provider_mappings[provider.mappingKey]];
    });
    if (!unconfigured.length || !state.providers.length) return;
    var notice = element("div", "notice");
    notice.appendChild(icon("alert"));
    notice.appendChild(element("span", "", unconfigured.length + " 个提供商尚未选择余额查询类型，可在“查询设置”中配置。"));
    target.appendChild(notice);
  }

  function renderResults() {
    var target = byID("results");
    target.setAttribute("aria-busy", "false");
    renderNotice();
    updateSummary();
    if (!state.providers.length) {
      setText(byID("query-meta"), "未发现 OpenAI 兼容提供商");
      emptyState(target, "暂无兼容提供商", "请先在 CPA 的 AI 提供商页面添加 OpenAI 兼容提供商。", null, null);
      return;
    }
    var activeProviders = state.providers.filter(function (provider) { return !provider.disabled; });
    if (!activeProviders.length) {
      setText(byID("query-meta"), "没有启用的兼容提供商");
      emptyState(target, "所有兼容提供商均已停用", "请先在 CPA 的 AI 提供商页面启用至少一个 OpenAI 兼容提供商。", null, null);
      return;
    }
    var accounts = buildAccounts();
    if (!accounts.length) {
      setText(byID("query-meta"), "尚无可查询账户");
      emptyState(target, "尚未完成查询设置", "请为提供商选择查询类型，并确认至少配置了一个接口密钥。", "前往查询设置", function () { setView("settings"); });
      return;
    }
    if (!state.results.length) {
      setText(byID("query-meta"), accounts.length + " 个账户等待查询");
      emptyState(target, "暂无查询结果", "点击“刷新余额”获取最新余额与套餐配额。", null, null);
      return;
    }
    setText(byID("query-meta"), state.results.length + " 个账户 · 更新于 " + (formatTime(state.results[0] && state.results[0].fetched_at) || "刚刚"));
    target.textContent = "";
    var grid = element("div", "result-grid");
    state.results.forEach(function (result, index) { grid.appendChild(resultCard(result, index)); });
    target.appendChild(grid);
  }

  function queryBalances(refresh) {
    var accounts = buildAccounts();
    if (!accounts.length) { state.results = []; renderResults(); return Promise.resolve(); }
    state.querying = true;
    setButtonBusy(byID("refresh-button"), true, "查询中");
    showSkeletons();
    setText(byID("query-meta"), "正在查询 " + accounts.length + " 个账户");
    return apiFetch("/balance-query/query", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ accounts: accounts, refresh: Boolean(refresh) })
    }, 60000).then(function (data) {
      state.results = Array.isArray(data.results) ? data.results : [];
      renderResults();
    }).catch(function (error) {
      state.results = [];
      renderResults();
      toast(error && error.message ? error.message : "余额查询失败", true);
    }).finally(function () {
      state.querying = false;
      setButtonBusy(byID("refresh-button"), false, "刷新余额");
    });
  }

  function keyChips(provider) {
    var wrap = element("div", "key-list");
    if (!provider.keys.length) {
      wrap.appendChild(element("span", "badge muted", "未配置密钥"));
      return wrap;
    }
    provider.keys.slice(0, 3).forEach(function (entry) { wrap.appendChild(element("span", "key-chip", maskKey(entry.apiKey))); });
    if (provider.keys.length > 3) wrap.appendChild(element("span", "badge muted", "另有 " + (provider.keys.length - 3) + " 个"));
    return wrap;
  }

  function renderSettings() {
    var body = byID("settings-body");
    var empty = byID("settings-empty");
    body.textContent = "";
    byID("ttl-input").value = String(state.config.cache_ttl_seconds);
    if (!state.providers.length) {
      empty.hidden = false;
      emptyState(empty, "暂无兼容提供商", "请先在 CPA 的 AI 提供商页面添加 OpenAI 兼容提供商。", null, null);
      return;
    }
    empty.hidden = true;
    state.providers.forEach(function (provider) {
      var row = document.createElement("tr");
      var providerCell = element("td", "provider-cell");
      var titleLine = element("div", "provider-title-line");
      titleLine.appendChild(element("span", "provider-title", provider.name));
      if (provider.disabled) titleLine.appendChild(element("span", "provider-disabled", "已停用"));
      providerCell.appendChild(titleLine);
      providerCell.appendChild(element("div", "provider-base", provider.baseUrl || "未设置接口地址"));
      row.appendChild(providerCell);
      var keysCell = document.createElement("td");
      keysCell.appendChild(keyChips(provider));
      row.appendChild(keysCell);
      var queryCell = element("td", "query-cell");
      var select = document.createElement("select");
      select.setAttribute("aria-label", provider.name + " 的余额查询类型");
      var blank = document.createElement("option");
      blank.value = "";
      blank.textContent = "不查询";
      select.appendChild(blank);
      PROVIDER_DEFINITIONS.forEach(function (definition) {
        var option = document.createElement("option");
        option.value = definition.value;
        option.textContent = definition.label;
        select.appendChild(option);
      });
      select.value = state.draftMappings[provider.mappingKey] || "";
      select.addEventListener("change", function () {
        if (select.value) state.draftMappings[provider.mappingKey] = select.value;
        else delete state.draftMappings[provider.mappingKey];
        state.dirty = true;
        setText(byID("save-state"), "有未保存的更改");
      });
      queryCell.appendChild(select);
      row.appendChild(queryCell);
      body.appendChild(row);
    });
  }

  function validateTTL() {
    var value = Number(byID("ttl-input").value);
    if (!Number.isInteger(value) || value < 0 || (value > 0 && value < 10) || value > 86400) return null;
    return value;
  }

  function saveSettings() {
    if (state.saving) return;
    var ttl = validateTTL();
    if (ttl == null) { toast("缓存时长应为 0，或 10 至 86400 之间的整数", true); byID("ttl-input").focus(); return; }
    var nextMappings = {};
    state.providers.forEach(function (provider) {
      var selected = state.draftMappings[provider.mappingKey];
      if (providerLabels[selected]) nextMappings[provider.mappingKey] = selected;
    });
    state.saving = true;
    setButtonBusy(byID("save-button"), true, "保存中");
    setText(byID("save-state"), "正在保存");
    apiFetch("/plugins/balance-query/config", {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ cache_ttl_seconds: ttl, provider_mappings: nextMappings })
    }).then(function () {
      state.config = { cache_ttl_seconds: ttl, provider_mappings: Object.assign({}, nextMappings) };
      state.draftMappings = Object.assign({}, nextMappings);
      state.dirty = false;
      state.results = [];
      setText(byID("save-state"), "已保存");
      toast("查询设置已保存", false);
      updateSummary();
      window.setTimeout(function () { if (!state.dirty) setText(byID("save-state"), ""); }, 2000);
    }).catch(function (error) {
      setText(byID("save-state"), "保存失败");
      toast(error && error.message ? error.message : "保存设置失败", true);
    }).finally(function () {
      state.saving = false;
      setButtonBusy(byID("save-button"), false, "保存设置");
    });
  }

  function loadData() {
    showApp();
    showSkeletons();
    setText(byID("query-meta"), "正在读取 OpenAI 兼容提供商");
    return Promise.all([
      apiFetch("/openai-compatibility"),
      apiFetch("/plugins/balance-query/config"),
      apiFetch("/proxy-url")
    ]).then(function (responses) {
      var list = responses[0] && responses[0]["openai-compatibility"];
      state.providers = (Array.isArray(list) ? list : []).map(normalizeProvider);
      state.config = normalizeConfig(responses[1]);
      state.globalProxyUrl = String(responses[2] && responses[2]["proxy-url"] || "").trim();
      state.draftMappings = Object.assign({}, state.config.provider_mappings);
      state.results = [];
      state.dirty = false;
      renderSettings();
      updateSummary();
      renderResults();
      return queryBalances(false);
    }).catch(function (error) {
      var authError = error && (error.status === 401 || error.status === 403);
      state.credentials.managementKey = "";
      showConnection(authError ? "管理密钥无效，请重新输入。" : (error && error.message ? error.message : "无法连接 CPA。"));
    });
  }

  document.querySelectorAll(".segment").forEach(function (button) {
    button.addEventListener("click", function () { setView(button.getAttribute("data-view")); });
  });
  byID("refresh-button").addEventListener("click", function () { if (!state.querying) queryBalances(true); });
  byID("save-button").addEventListener("click", saveSettings);
  byID("reconnect-button").addEventListener("click", function () { showConnection(""); });
  byID("ttl-input").addEventListener("input", function () {
    state.dirty = Number(byID("ttl-input").value) !== state.config.cache_ttl_seconds || JSON.stringify(state.draftMappings) !== JSON.stringify(state.config.provider_mappings);
    setText(byID("save-state"), state.dirty ? "有未保存的更改" : "");
  });
  byID("connection-form").addEventListener("submit", function (event) {
    event.preventDefault();
    var base = normalizeApiBase(byID("api-base-input").value);
    var key = byID("management-key-input").value.trim();
    if (!base) { setText(byID("connection-error"), "请输入有效的 CPA HTTP(S) 地址。" ); return; }
    if (!key) { setText(byID("connection-error"), "请输入管理密钥。" ); return; }
    state.credentials = { apiBase: base, managementKey: key };
    byID("management-key-input").value = "";
    setText(byID("connection-error"), "");
    setButtonBusy(byID("connect-button"), true, "连接中");
    loadData().finally(function () { setButtonBusy(byID("connect-button"), false, "连接"); });
  });
  window.addEventListener("beforeunload", function (event) {
    if (!state.dirty) return;
    event.preventDefault();
    event.returnValue = "";
  });

  var restored = restoreCredentials();
  if (restored) {
    state.credentials = restored;
    loadData();
  } else {
    showConnection("");
  }
})();
</script>
</body>
</html>`
