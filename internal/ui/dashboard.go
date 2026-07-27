// Package ui renders the balance query management page.
package ui

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/Hamster-Prime/balance-query/internal/balance"
)

type providerDefinition struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Status      string `json:"status,omitempty"`
	Description string `json:"description,omitempty"`
}

// RenderDashboard renders a self-contained page for OpenAI-compatible provider
// balance mappings and queries. Secrets are loaded by the page at runtime and
// are never interpolated into the generated HTML.
func RenderDashboard(ttlSeconds int) []byte {
	definitions := make([]providerDefinition, 0, len(balance.AllProviders()))
	for _, providerType := range balance.AllProviders() {
		definition := providerDefinition{
			Value:  string(providerType),
			Label:  balance.ProviderLabel[providerType],
			Status: "available",
		}
		switch providerType {
		case balance.ProviderMiniMaxAPI, balance.ProviderXiaomiAPI, balance.ProviderXiaomiToken,
			balance.ProviderLongcat, balance.ProviderOpenCode, balance.ProviderVolcengine:
			definition.Status = "console_only"
			definition.Description = "官方未提供模型 API Key 余额查询接口，仅能在官网登录控制台查看。"
		}
		definitions = append(definitions, definition)
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
<meta id="theme-color" name="theme-color" content="#faf9f5">
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
    "--glass-bg-secondary","--glass-border","--motion-fast","--motion-normal",
    "--motion-enter"
  ];
  var root = document.documentElement;
  var media = window.matchMedia ? window.matchMedia("(prefers-color-scheme: dark)") : null;
  var parentOrigin = "";
  try { parentOrigin = document.referrer ? new URL(document.referrer).origin : ""; } catch (_) {}

  function normalizeTheme(value) {
    value = String(value || "").toLowerCase();
    if (value === "dark" || value === "white" || value === "light") return value;
    if (value === "auto" || value === "system") return media && media.matches ? "dark" : "white";
    return "light";
  }

  function applyTheme(value) {
    var theme = normalizeTheme(value);
    if (theme === "light") root.removeAttribute("data-theme");
    else root.setAttribute("data-theme", theme);
    root.style.colorScheme = theme === "dark" ? "dark" : "light";
    root.dataset.resolvedTheme = theme === "dark" ? "dark" : "light";
    var themeColor = document.getElementById("theme-color");
    if (themeColor) themeColor.setAttribute("content", theme === "dark" ? "#151412" : theme === "white" ? "#ffffff" : "#faf9f5");
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

  function inferredParentTheme(parentRoot, parentBody, computed, bodyComputed) {
    var explicit = parentRoot.getAttribute("data-theme");
    if (!explicit && parentBody) explicit = parentBody.getAttribute("data-theme");
    if (explicit === "dark" || explicit === "white" || explicit === "light") return explicit;
    var className = String(parentRoot.className || "") + " " + String(parentBody && parentBody.className || "");
    className = className.toLowerCase();
    if (className.indexOf("dark") !== -1) return "dark";
    if (className.indexOf("white") !== -1) return "white";
    var surface = computed && computed.getPropertyValue("--bg-secondary").trim().toLowerCase();
    if (!surface && bodyComputed) surface = bodyComputed.getPropertyValue("--bg-secondary").trim().toLowerCase();
    if (surface === "#151412" || surface === "rgb(21, 20, 18)") return "dark";
    if (surface === "#ffffff" || surface === "rgb(255, 255, 255)") return "white";
    return "light";
  }

  function copyParentTheme(parentRoot) {
    if (!parentRoot) return false;
    try {
      var parentBody = window.parent.document.body;
      var computed = window.parent.getComputedStyle(parentRoot);
      var bodyComputed = parentBody ? window.parent.getComputedStyle(parentBody) : null;
      applyTheme(inferredParentTheme(parentRoot, parentBody, computed, bodyComputed));
      TOKENS.forEach(function (token) {
        var value = computed.getPropertyValue(token);
        if ((!value || !value.trim()) && bodyComputed) value = bodyComputed.getPropertyValue(token);
        if (value && value.trim()) root.style.setProperty(token, value.trim());
      });
    } catch (_) {
      applyTheme(parentRoot.getAttribute("data-theme") || "light");
    }
    return true;
  }

  var parentRoot = sameOriginParentRoot();
  var queryTheme = "";
  try {
    var query = new URLSearchParams(window.location.search);
    queryTheme = query.get("theme") || query.get("data-theme") || "";
  } catch (_) {}
  if (!copyParentTheme(parentRoot)) applyTheme(queryTheme || readStoredTheme());

  if (parentRoot && window.MutationObserver) {
    var syncTheme = function () { copyParentTheme(parentRoot); };
    var observer = new MutationObserver(syncTheme);
    observer.observe(parentRoot, {
      attributes: true,
      attributeFilter: ["data-theme", "class", "style"]
    });
    try {
      observer.observe(window.parent.document.body, {
        attributes: true,
        attributeFilter: ["data-theme", "class", "style"]
      });
    } catch (_) {}
    window.setInterval(syncTheme, 1000);
  }

  function onMediaChange() {
    if (!sameOriginParentRoot() && readStoredTheme() === "auto") applyTheme("auto");
  }
  if (media && media.addEventListener) media.addEventListener("change", onMediaChange);

  window.addEventListener("storage", function (event) {
    if (event.key === THEME_KEY && !sameOriginParentRoot()) applyTheme(readStoredTheme());
  });

  window.addEventListener("message", function (event) {
    if (event.source !== window.parent || (event.origin !== window.location.origin && event.origin !== parentOrigin)) return;
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

  try {
    if (window.parent !== window) {
      window.parent.postMessage({ type: "balance-query:theme-request" }, parentOrigin || window.location.origin);
    }
  } catch (_) {}
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
  --motion-fast:150ms ease;
  --motion-normal:300ms ease;
  --motion-enter:360ms cubic-bezier(.25,1,.5,1);
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
html,body{margin:0;min-width:280px;min-height:100%;background:var(--bg-secondary);color:var(--text-primary);transition:background-color var(--motion-normal),color var(--motion-normal)}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI","Roboto","Oxygen","Ubuntu","Cantarell","Helvetica Neue",sans-serif;font-size:14px;line-height:1.5}
button,input,select{font:inherit;letter-spacing:0}
button{color:inherit}
[hidden]{display:none!important}
::-webkit-scrollbar{width:8px;height:8px}
::-webkit-scrollbar-track{background:var(--bg-secondary)}
::-webkit-scrollbar-thumb{background:var(--border-color);border-radius:9999px}
::-webkit-scrollbar-thumb:hover{background:var(--border-hover)}
.icon{width:16px;height:16px;display:block;flex:0 0 auto;fill:none;stroke:currentColor;stroke-width:2;stroke-linecap:round;stroke-linejoin:round}
.app{width:100%;max-width:1440px;margin:0 auto;padding:28px clamp(18px,3vw,44px) 48px;animation:page-in var(--motion-enter) both}
.masthead{display:flex;align-items:flex-start;justify-content:space-between;gap:20px;padding:20px 22px;border:1px solid var(--glass-border);border-radius:12px;background:linear-gradient(145deg,color-mix(in srgb,var(--bg-primary) 88%,transparent),color-mix(in srgb,var(--bg-secondary) 72%,transparent));backdrop-filter:var(--glass-backdrop-filter);-webkit-backdrop-filter:var(--glass-backdrop-filter);box-shadow:var(--shadow)}
.brand{display:flex;align-items:center;gap:12px;min-width:0}
.brand-mark{width:42px;height:42px;display:grid;place-items:center;flex:0 0 auto;border:1px solid var(--primary-30);border-radius:10px;background:var(--primary-8);color:var(--text-primary);box-shadow:var(--shadow)}
.brand-mark .icon{width:20px;height:20px}
h1{font-size:22px;line-height:1.25;font-weight:650;margin:0;color:var(--text-primary)}
.subtitle{margin:3px 0 0;color:var(--text-secondary);font-size:13px}
.head-state{display:flex;align-items:center;gap:8px;color:var(--text-secondary);font-size:13px;white-space:nowrap;padding-top:8px}
.state-dot{width:7px;height:7px;border-radius:50%;background:var(--success-color);box-shadow:0 0 0 3px color-mix(in srgb,var(--success-color) 14%,transparent)}
.state-dot.pending{background:var(--quota-medium-color);box-shadow:0 0 0 3px var(--amber-10)}
.workspace-nav{display:flex;align-items:center;justify-content:space-between;gap:16px;padding:18px 0 16px}
.segments{display:inline-grid;grid-template-columns:repeat(2,minmax(116px,1fr));padding:3px;border:1px solid var(--border-color);border-radius:8px;background:var(--bg-tertiary)}
.segment{height:34px;border:0;border-radius:6px;background:transparent;color:var(--text-secondary);padding:0 12px;display:inline-flex;align-items:center;justify-content:center;gap:7px;cursor:pointer;font-weight:600;font-size:13px;transition:background var(--motion-fast),color var(--motion-fast),box-shadow var(--motion-fast)}
.segment:hover{color:var(--text-primary)}
.segment[aria-selected="true"]{background:var(--bg-primary);color:var(--text-primary);box-shadow:var(--shadow)}
.toolbar{display:flex;align-items:center;gap:8px;flex-wrap:wrap;justify-content:flex-end}
.btn{height:36px;border:1px solid transparent;border-radius:8px;padding:0 12px;display:inline-flex;align-items:center;justify-content:center;gap:7px;cursor:pointer;font-weight:600;font-size:13px;transition:background var(--motion-fast),border-color var(--motion-fast),color var(--motion-fast),transform var(--motion-fast),box-shadow var(--motion-fast)}
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
.summary{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:10px;margin-bottom:20px}
.summary-item{padding:15px 17px;min-width:0;border:1px solid var(--border-color);border-radius:10px;background:var(--bg-primary);box-shadow:var(--shadow);transition:border-color var(--motion-fast),transform var(--motion-fast),box-shadow var(--motion-fast)}
.summary-item:hover{border-color:var(--border-hover);transform:translateY(-1px);box-shadow:var(--shadow-lg)}
.summary-value{display:block;color:var(--text-primary);font-size:20px;font-weight:650;font-variant-numeric:tabular-nums;line-height:1.2}
.summary-label{display:block;color:var(--text-tertiary);font-size:12px;margin-top:4px}
.section-head{display:flex;align-items:flex-end;justify-content:space-between;gap:16px;margin:0 0 12px}
.section-title{font-size:15px;font-weight:650;margin:0;color:var(--text-primary)}
.section-meta{font-size:12px;color:var(--text-tertiary);margin:2px 0 0}
.result-grid{display:grid;grid-template-columns:minmax(0,1fr);gap:12px;align-items:start}
.result-card{min-width:0;padding:18px;border:1px solid var(--glass-border);border-radius:12px;background:linear-gradient(145deg,color-mix(in srgb,var(--bg-primary) 92%,transparent),color-mix(in srgb,var(--bg-secondary) 70%,transparent));backdrop-filter:var(--glass-backdrop-filter);-webkit-backdrop-filter:var(--glass-backdrop-filter);box-shadow:var(--shadow);animation:item-in 400ms ease-out both;transition:border-color var(--motion-fast),box-shadow var(--motion-fast),transform var(--motion-fast),background-color var(--motion-normal)}
.result-card:hover{border-color:var(--border-hover);box-shadow:var(--shadow-lg)}
.result-card.error{border-color:var(--failure-badge-border)}
.result-card.limited{border-color:var(--amber-30)}
.result-head{display:flex;align-items:flex-start;justify-content:space-between;gap:12px;min-width:0}
.result-actions{display:flex;align-items:center;justify-content:flex-end;gap:7px;flex:0 0 auto;flex-wrap:wrap}
.result-name{font-weight:650;color:var(--text-primary);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.result-url{font-family:"SFMono-Regular",Consolas,"Liberation Mono",monospace;color:var(--text-tertiary);font-size:11px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;margin-top:2px}
.badge{display:inline-flex;align-items:center;gap:5px;border-radius:9999px;border:1px solid var(--border-color);padding:3px 8px;font-size:11px;font-weight:600;white-space:nowrap}
.badge.success{color:var(--success-badge-text);background:var(--success-badge-bg);border-color:var(--success-badge-border)}
.badge.failure{color:var(--failure-badge-text);background:var(--failure-badge-bg);border-color:var(--failure-badge-border)}
.badge.warning{color:var(--amber-text);background:var(--amber-10);border-color:var(--amber-30)}
.badge.muted{color:var(--text-secondary);background:var(--bg-tertiary)}
.detail-toggle{height:28px;border:1px solid var(--border-color);border-radius:7px;background:var(--bg-primary);color:var(--text-secondary);padding:0 8px;display:inline-flex;align-items:center;justify-content:center;gap:5px;cursor:pointer;font-size:11px;font-weight:600;white-space:nowrap;transition:background var(--motion-fast),border-color var(--motion-fast),color var(--motion-fast),box-shadow var(--motion-fast)}
.detail-toggle:hover{background:var(--bg-hover);border-color:var(--border-hover);color:var(--text-primary)}
.detail-toggle:focus-visible{outline:2px solid var(--primary-color);outline-offset:2px}
.detail-toggle .icon{width:13px;height:13px;transition:transform var(--motion-normal)}
.detail-toggle[aria-expanded="true"] .icon{transform:rotate(180deg)}
.result-overview{display:flex;align-items:center;gap:8px 12px;flex-wrap:wrap;margin-top:14px}
.quota-main{font-size:20px;line-height:1.3;font-weight:680;color:var(--text-primary);margin:0;overflow-wrap:anywhere}
.quota-main.failure{font-size:14px;color:var(--warning-text);font-weight:600;line-height:1.45}
.error-detail{color:var(--text-secondary);font-size:12px;line-height:1.45;overflow-wrap:anywhere}
.progress-track{height:7px;border-radius:9999px;background:var(--bg-tertiary);overflow:hidden;margin-top:12px}
.progress-bar{height:100%;width:0;border-radius:inherit;background:var(--success-color);transition:width 300ms ease}
.progress-bar.medium{background:var(--quota-medium-color)}
.progress-bar.high{background:var(--error-color)}
.result-foot{display:flex;align-items:center;justify-content:space-between;gap:8px;margin-top:13px;color:var(--text-tertiary);font-size:11px}
.key-preview{font-family:"SFMono-Regular",Consolas,"Liberation Mono",monospace;color:var(--text-secondary)}
.account-meta{display:flex;gap:6px;flex-wrap:wrap;margin:0}
.detail{display:inline-flex;gap:4px;border-radius:6px;border:1px solid color-mix(in srgb,var(--border-color) 75%,transparent);background:var(--bg-tertiary);color:var(--text-secondary);padding:4px 7px;font-size:11px;max-width:100%;overflow-wrap:anywhere}
.detail-label{color:var(--text-tertiary)}
.quota-groups{display:flex;flex-direction:column;gap:12px;margin-top:15px}
.quota-group{border-top:1px solid var(--border-color);padding-top:12px}
.quota-group-head{display:flex;align-items:center;justify-content:space-between;gap:8px;margin-bottom:8px}
.quota-group-title{font-size:12px;font-weight:650;color:var(--text-secondary);margin:0;overflow-wrap:anywhere}
.quota-group-count{font-size:10px;color:var(--text-tertiary);border:1px solid var(--border-color);border-radius:9999px;padding:1px 6px;white-space:nowrap}
.quota-window-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:8px}
.quota-window{min-width:0;padding:11px 12px;border:1px solid var(--border-color);border-radius:9px;background:color-mix(in srgb,var(--bg-primary) 78%,var(--bg-secondary));transition:border-color var(--motion-fast),background-color var(--motion-normal)}
.quota-window.unlimited{border-color:color-mix(in srgb,var(--success-color) 38%,var(--border-color));background:color-mix(in srgb,var(--success-color) 6%,var(--bg-primary))}
.quota-window.unavailable{opacity:.72;background:var(--bg-tertiary)}
.quota-window-head{display:flex;align-items:center;justify-content:space-between;gap:8px;min-width:0}
.quota-window-label{font-size:12px;font-weight:650;color:var(--text-primary);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.quota-status{font-size:10px;color:var(--text-tertiary);white-space:nowrap}
.quota-status.good{color:var(--success-color)}
.quota-status.warn{color:var(--amber-text)}
.quota-window-value{display:flex;align-items:baseline;gap:5px;flex-wrap:wrap;margin-top:8px;color:var(--text-primary);font-variant-numeric:tabular-nums}
.quota-window-value strong{font-size:18px;line-height:1.2;font-weight:680}
.quota-window-value span{font-size:11px;color:var(--text-secondary)}
.quota-window .progress-track{height:5px;margin-top:9px}
.quota-window-meta{display:flex;align-items:center;justify-content:space-between;gap:8px;min-height:17px;margin-top:7px;color:var(--text-tertiary);font-size:10px;line-height:1.35}
.quota-window-meta span{overflow-wrap:anywhere}
.account-detail-collapse{display:grid;grid-template-rows:0fr;opacity:0;visibility:hidden;pointer-events:none;transition:grid-template-rows var(--motion-normal),opacity var(--motion-fast),visibility 0s linear var(--motion-normal)}
.account-detail-collapse[aria-hidden="false"]{grid-template-rows:1fr;opacity:1;visibility:visible;pointer-events:auto;transition:grid-template-rows var(--motion-normal),opacity var(--motion-fast),visibility 0s linear 0s}
.account-detail-inner{min-height:0;overflow:hidden}
.detail-section{border-top:1px solid var(--border-color);margin-top:13px;padding-top:11px}
.detail-section-title{font-size:11px;font-weight:650;color:var(--text-secondary);margin:0 0 7px}
.detail-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(200px,1fr));gap:6px}
.detail-row{min-width:0;padding:6px 8px;border-radius:7px;background:var(--bg-tertiary)}
.detail-row dt{font-size:10px;color:var(--text-tertiary);margin:0}
.detail-row dd{font-size:11px;color:var(--text-secondary);margin:2px 0 0;overflow-wrap:anywhere}
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
.query-help{display:flex;align-items:flex-start;gap:5px;margin-top:6px;color:var(--amber-text);font-size:10px;line-height:1.4}
.query-help .icon{width:12px;height:12px;margin-top:1px}
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
.skeleton-grid{display:grid;grid-template-columns:minmax(0,1fr);gap:12px}
.skeleton-card{height:150px;border:1px solid var(--border-color);border-radius:12px;background:var(--bg-primary);padding:18px;overflow:hidden}
.skeleton{position:relative;overflow:hidden;background:var(--bg-tertiary);border-radius:5px}
.skeleton::after{content:"";position:absolute;inset:0;transform:translateX(-100%);background:linear-gradient(90deg,transparent,color-mix(in srgb,var(--text-primary) 7%,transparent),transparent);animation:skeleton 1.5s ease-in-out infinite}
.skeleton-line{height:12px;margin-bottom:10px}.skeleton-line.short{width:42%}.skeleton-line.medium{width:68%}.skeleton-line.value{width:54%;height:24px;margin-top:25px}
.toast-region{position:fixed;z-index:20;right:18px;bottom:18px;display:flex;flex-direction:column;gap:8px;pointer-events:none}
.toast{max-width:min(390px,calc(100vw - 36px));display:flex;align-items:flex-start;gap:8px;padding:10px 12px;border:1px solid var(--border-color);border-radius:8px;background:var(--floating-surface);box-shadow:var(--floating-shadow);color:var(--text-primary);font-size:13px;animation:toast-in 300ms ease-out both}
.toast.error{border-color:var(--failure-badge-border)}
@keyframes spin{to{transform:rotate(360deg)}}
@keyframes skeleton{100%{transform:translateX(100%)}}
@keyframes page-in{from{opacity:0;transform:translate3d(0,28px,0)}to{opacity:1;transform:translate3d(0,0,0)}}
@keyframes item-in{from{opacity:0;transform:translateY(6px)}to{opacity:1;transform:translateY(0)}}
@keyframes toast-in{from{opacity:0;transform:translateY(8px)}to{opacity:1;transform:translateY(0)}}
@media (max-width:760px){
  .app{padding:16px 14px 28px}.masthead{align-items:center}.subtitle{max-width:230px}.head-state{display:none}
  .workspace-nav{align-items:stretch;flex-direction:column}.segments{width:100%}.toolbar{justify-content:stretch}.toolbar .btn{flex:1}
  .summary{grid-template-columns:repeat(2,minmax(0,1fr))}
  .result-grid,.skeleton-grid{grid-template-columns:1fr}
  .quota-window-grid,.detail-grid{grid-template-columns:1fr}
  .result-head{align-items:stretch;flex-direction:column}.result-actions{justify-content:space-between}.result-actions .badge{margin-right:auto}
  .settings-toolbar{align-items:flex-start;flex-direction:column}.ttl-field{width:100%;justify-content:space-between}
  table,thead,tbody,tr,th,td{display:block}thead{display:none}table{table-layout:auto}tbody tr{padding:13px 14px;border-bottom:1px solid var(--border-color)}tbody tr:last-child{border-bottom:0}td{padding:0;border:0}td+td{margin-top:10px}.provider-base{white-space:normal;overflow-wrap:anywhere}.query-cell::before{content:"余额查询类型";display:block;color:var(--text-tertiary);font-size:11px;margin-bottom:5px}
}
@media (max-width:420px){.btn-label.optional{display:none}.summary-item{padding:12px}.result-card{padding:14px}.masthead{padding:16px}.connection-shell{padding:14px}.connection-card{padding:18px}}
@media (prefers-reduced-motion:reduce){*,*::before,*::after{scroll-behavior:auto!important;animation-duration:.001ms!important;animation-iteration-count:1!important;transition-duration:.001ms!important}.btn:hover:not(:disabled),.result-card:hover,.summary-item:hover{transform:none}}
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
  <symbol id="i-chevron" viewBox="0 0 24 24"><path d="m6 9 6 6 6-6"/></symbol>
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
  var providerDefinitions = Object.create(null);
  PROVIDER_DEFINITIONS.forEach(function (item) {
    providerLabels[item.value] = item.label;
    providerDefinitions[item.value] = item;
  });

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

  function formatDateTime(value) {
    if (!value) return "";
    var date = new Date(value);
    if (Number.isNaN(date.getTime())) return redactSecrets(value);
    return new Intl.DateTimeFormat("zh-CN", {
      month:"numeric", day:"numeric", hour:"2-digit", minute:"2-digit"
    }).format(date);
  }

  function owns(object, key) { return Boolean(object) && Object.prototype.hasOwnProperty.call(object, key); }
  function finiteNumber(value) {
    var number = Number(value);
    return Number.isFinite(number) ? number : null;
  }
  function clampPercent(value) { return Math.max(0, Math.min(100, value)); }

  function unitLabel(unit) {
    var raw = String(unit || "").trim();
    var labels = {
      usd:"美元",cny:"元",rmb:"元",dollar:"美元",dollars:"美元",
      token:"令牌",tokens:"令牌",request:"次",requests:"次",call:"次",calls:"次",
      percent:"%",percentage:"%",count:"次",times:"次",credit:"额度",credits:"额度"
    };
    return labels[raw.toLowerCase()] || raw || "额度";
  }

  function formatAmount(value, unit) {
    var number = finiteNumber(value);
    if (number == null) return "—";
    var normalizedUnit = String(unit || "").trim().toLowerCase();
    var maximumFractionDigits = Math.abs(number) >= 100 ? 2 : 4;
    var formatted = number.toLocaleString("zh-CN", { maximumFractionDigits:maximumFractionDigits });
    if (normalizedUnit === "usd" || normalizedUnit === "dollar" || normalizedUnit === "dollars") return "$" + formatted;
    if (normalizedUnit === "cny" || normalizedUnit === "rmb") return "¥" + formatted;
    if (normalizedUnit === "%" || normalizedUnit === "percent" || normalizedUnit === "percentage") return formatted + "%";
    return formatted;
  }

  function translateWindowLabel(value) {
    var raw = String(value || "").trim();
    if (!raw) return "配额周期";
    var lower = raw.toLowerCase().replace(/[_-]+/g, " ").replace(/\s+/g, " ").trim();
    var hour = lower.match(/^(\d+(?:\.\d+)?)\s*h(?:our)?s?(?:\s+(?:limit|window|quota))?$/);
    if (hour) return hour[1] + " 小时配额";
    var day = lower.match(/^(\d+(?:\.\d+)?)\s*d(?:ay)?s?(?:\s+(?:limit|window|quota))?$/);
    if (day) return day[1] === "1" ? "日配额" : day[1] + " 天配额";
    var labels = {
      daily:"日配额",day:"日配额","daily limit":"日配额","daily quota":"日配额",
      weekly:"周配额",week:"周配额","weekly limit":"周配额","weekly quota":"周配额","7d":"周配额","7 day":"周配额","7 days":"周配额",
      monthly:"月配额",month:"月配额","monthly limit":"月配额","monthly quota":"月配额",
      hourly:"小时配额",primary:"主要配额",secondary:"次级配额",rolling:"滚动周期",interval:"当前周期",
      "current interval":"当前周期","current window":"当前周期"
    };
    return labels[lower] || raw
      .replace(/\bweekly\b/ig, "周")
      .replace(/\bmonthly\b/ig, "月")
      .replace(/\bdaily\b/ig, "日")
      .replace(/\bhourly\b/ig, "小时")
      .replace(/\blimit\b/ig, "配额")
      .replace(/\bquota\b/ig, "配额")
      .replace(/\bwindow\b/ig, "周期");
  }

  function translateDisplayText(value) {
    var raw = String(value || "").trim();
    var labels = {
      "coding plan":"编程套餐","token plan":"令牌套餐","subscription plan":"订阅套餐",
      "free plan":"免费套餐","professional plan":"专业套餐","enterprise plan":"企业套餐",
      "pay as you go":"按量付费","unrestricted":"不限量","wallet balance":"钱包余额"
    };
    return labels[raw.toLowerCase()] || raw;
  }

  function translateStatus(value) {
    var raw = String(value || "").trim();
    if (!raw) return "";
    var labels = {
      active:"可用",available:"可用",normal:"正常",ok:"正常",success:"正常",
      unlimited:"不限量",inactive:"未启用",unavailable:"不可用",disabled:"已停用",
      exhausted:"已用尽",depleted:"已用尽",expired:"已过期",unsupported:"暂不支持",
      outside_plan:"不在套餐内","outside plan":"不在套餐内",boosted:"已加成"
    };
    return labels[raw.toLowerCase()] || raw;
  }

  function durationText(seconds) {
    var remaining = Math.max(0, Math.floor(Number(seconds) || 0));
    if (!remaining) return "";
    var days = Math.floor(remaining / 86400);
    var hours = Math.floor(remaining % 86400 / 3600);
    var minutes = Math.floor(remaining % 3600 / 60);
    if (days) return days + " 天 " + hours + " 小时";
    if (hours) return hours + " 小时 " + minutes + " 分";
    if (minutes) return minutes + " 分";
    return remaining + " 秒";
  }

  function formatBalance(result) {
    if (result.quota_display) return redactSecrets(result.quota_display);
    if (owns(result, "balance_usd")) {
      var amount = Number(result.balance_usd);
      if (Number.isFinite(amount)) return "$" + amount.toFixed(amount >= 100 ? 2 : 4);
    }
    if (Number(result.tokens_total) > 0) {
      return Number(result.tokens_remaining || 0).toLocaleString("zh-CN") + " 可用令牌";
    }
    var windowCount = Array.isArray(result.quota_windows) ? result.quota_windows.length : 0;
    if (windowCount) return windowCount + " 个配额周期";
    var detailCount = result.extra && typeof result.extra === "object" ? Object.keys(result.extra).length : 0;
    if (detailCount) return detailCount + " 项账户详情";
    return "暂无可展示的配额数值";
  }

  function detailLabel(key) {
    var labels = {
      plan:"套餐",reset:"重置时间",currency:"币种",remaining:"剩余",used:"已使用",
      total:"总量",limit:"额度",requests:"请求次数",window:"统计周期",expires:"到期时间",
      expires_at:"到期时间",balance:"余额",status:"状态",mode:"计费模式",plan_name:"套餐名称",
      today_requests:"今日请求数",total_requests:"累计请求数",today_tokens:"今日令牌数",total_tokens:"累计令牌数",
      input_tokens:"输入令牌",output_tokens:"输出令牌",cache_creation_tokens:"缓存写入令牌",cache_read_tokens:"缓存读取令牌",
      today_cost:"今日费用",total_cost:"累计费用",actual_cost:"实际费用",average_duration_ms:"平均耗时",
      rpm:"每分钟请求数",tpm:"每分钟令牌数",daily_usage:"日用量",weekly_usage:"周用量",monthly_usage:"月用量",
      daily_limit:"日额度",weekly_limit:"周额度",monthly_limit:"月额度",booster_balance:"加量包余额",
      monthly_charge_limit:"月度扣费上限",monthly_used:"本月已扣费",days_until_expiry:"距到期天数"
    };
    var normalized = String(key || "").replace(/([a-z])([A-Z])/g, "$1_$2").toLowerCase();
    if (labels[normalized]) return labels[normalized];
    var segments = {
      today:"今日",daily:"日",weekly:"周",monthly:"月",total:"累计",average:"平均",
      input:"输入",output:"输出",cache:"缓存",creation:"写入",read:"读取",actual:"实际",
      usage:"用量",used:"已用",remaining:"剩余",limit:"额度",quota:"配额",balance:"余额",
      request:"请求",requests:"请求数",token:"令牌",tokens:"令牌数",cost:"费用",duration:"耗时",
      expires:"到期",expiry:"到期",days:"天数",status:"状态",count:"数量",amount:"金额",
      model:"模型",name:"名称",window:"周期",start:"开始",end:"结束",time:"时间"
    };
    var translated = normalized.split(/[_\s-]+/).map(function (part) { return segments[part] || part.toUpperCase(); }).join("");
    return translated || "详情";
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

  function localizedError(value) {
    return redactSecrets(value)
      .replace(/unauthorized/ig, "认证失败")
      .replace(/forbidden/ig, "权限不足")
      .replace(/invalid api[ -]?key/ig, "接口密钥无效")
      .replace(/not found/ig, "接口不存在")
      .replace(/request timeout|timed out/ig, "请求超时")
      .replace(/connection refused/ig, "连接被拒绝");
  }

  function quotaPercent(item) {
    var explicitUsed = finiteNumber(item.used_percent);
    if (owns(item, "used_percent") && explicitUsed != null) return clampPercent(explicitUsed);
    var explicitRemaining = finiteNumber(item.remaining_percent);
    if (owns(item, "remaining_percent") && explicitRemaining != null) return clampPercent(100 - explicitRemaining);
    var total = finiteNumber(item.total);
    var used = finiteNumber(item.used);
    var remaining = finiteNumber(item.remaining);
    if (total != null && total > 0 && owns(item, "used") && used != null) return clampPercent(used / total * 100);
    if (total != null && total > 0 && owns(item, "remaining") && remaining != null) return clampPercent((total - remaining) / total * 100);
    return null;
  }

  function quotaRemaining(item) {
    var remaining = finiteNumber(item.remaining);
    if (owns(item, "remaining") && remaining != null) return remaining;
    var total = finiteNumber(item.total);
    var used = finiteNumber(item.used);
    if (total != null && used != null && owns(item, "used")) return Math.max(0, total - used);
    return null;
  }

  function quotaWindowRank(item) {
    var label = String(item && item.label || "").toLowerCase();
    if (/\b\d+(?:\.\d+)?\s*h/.test(label) || label.indexOf("小时") !== -1) return 10;
    if (label === "1d" || label.indexOf("daily") !== -1 || label.indexOf("日配额") !== -1) return 20;
    if (label === "7d" || label.indexOf("weekly") !== -1 || label.indexOf("周配额") !== -1) return 30;
    if (label.indexOf("monthly") !== -1 || label.indexOf("月配额") !== -1) return 40;
    return 50;
  }

  function quotaResetNode(item) {
    var resetIn = finiteNumber(item.reset_in_seconds);
    var resetAt = item.reset_at;
    var textValue = "";
    var absolute = 0;
    if (resetIn != null && resetIn > 0) {
      textValue = durationText(resetIn) + "后重置";
      absolute = Date.now() + resetIn * 1000;
    } else if (resetAt) {
      var parsed = new Date(resetAt);
      if (!Number.isNaN(parsed.getTime()) && parsed.getTime() > Date.now()) {
        absolute = parsed.getTime();
        textValue = durationText((absolute - Date.now()) / 1000) + "后重置";
      } else {
        textValue = "重置于 " + formatDateTime(resetAt);
      }
    }
    if (!textValue) return null;
    var node = element("span", "quota-reset", textValue);
    if (absolute) node.setAttribute("data-reset-at", String(absolute));
    return node;
  }

  function quotaWindowCard(item) {
    var unavailable = Boolean(item.unavailable);
    var unlimited = Boolean(item.unlimited);
    var box = element("div", "quota-window" + (unlimited ? " unlimited" : "") + (unavailable ? " unavailable" : ""));
    var head = element("div", "quota-window-head");
    head.appendChild(element("span", "quota-window-label", translateWindowLabel(item.label)));
    var status = unlimited ? "不限量" : unavailable ? "不可用" : translateStatus(item.status);
    if (status) head.appendChild(element("span", "quota-status" + (unlimited ? " good" : unavailable ? " warn" : ""), status));
    box.appendChild(head);

    var value = element("div", "quota-window-value");
    var unit = unitLabel(item.unit);
    var total = finiteNumber(item.total);
    var used = finiteNumber(item.used);
    var remaining = quotaRemaining(item);
    var remainingPercent = finiteNumber(item.remaining_percent);
    var usedPercent = quotaPercent(item);
    if (unlimited) {
      value.appendChild(element("strong", "", "不限量"));
      value.appendChild(element("span", "", "当前周期"));
    } else if (unavailable) {
      value.appendChild(element("strong", "", "—"));
      value.appendChild(element("span", "", "当前套餐不可用"));
    } else if (remaining != null) {
      value.appendChild(element("strong", "", formatAmount(remaining, item.unit)));
      value.appendChild(element("span", "", total != null && total > 0 ? "/ " + formatAmount(total, item.unit) + " " + unit + "剩余" : unit + "剩余"));
    } else if (owns(item, "remaining_percent") && remainingPercent != null) {
      value.appendChild(element("strong", "", formatAmount(remainingPercent, "%")));
      value.appendChild(element("span", "", "剩余"));
    } else if (usedPercent != null) {
      value.appendChild(element("strong", "", formatAmount(100 - usedPercent, "%")));
      value.appendChild(element("span", "", "剩余"));
    } else {
      value.appendChild(element("strong", "", "—"));
      value.appendChild(element("span", "", "暂无数值"));
    }
    box.appendChild(value);

    if (!unlimited && !unavailable && usedPercent != null) {
      var track = element("div", "progress-track");
      track.setAttribute("role", "progressbar");
      track.setAttribute("aria-label", translateWindowLabel(item.label) + "使用进度");
      track.setAttribute("aria-valuemin", "0");
      track.setAttribute("aria-valuemax", "100");
      track.setAttribute("aria-valuenow", String(Math.round(usedPercent)));
      var bar = element("div", "progress-bar" + (usedPercent >= 85 ? " high" : usedPercent >= 60 ? " medium" : ""));
      track.appendChild(bar);
      box.appendChild(track);
      window.requestAnimationFrame(function () { bar.style.width = usedPercent.toFixed(1) + "%"; });
    }

    var meta = element("div", "quota-window-meta");
    var usageText = "";
    if (owns(item, "used") && used != null) usageText = "已用 " + formatAmount(used, item.unit) + (unit === "%" ? "" : " " + unit);
    else if (usedPercent != null) usageText = "已用 " + formatAmount(usedPercent, "%");
    meta.appendChild(element("span", "", usageText));
    var resetNode = quotaResetNode(item);
    if (resetNode) meta.appendChild(resetNode);
    if (usageText || resetNode) box.appendChild(meta);
    return box;
  }

  function renderQuotaGroups(card, result) {
    var windows = Array.isArray(result.quota_windows) ? result.quota_windows.filter(function (item) { return item && typeof item === "object"; }) : [];
    if (!windows.length) return false;
    var groups = [];
    var groupByName = Object.create(null);
    windows.forEach(function (item) {
      var name = String(item.group || "").trim() || "配额周期";
      if (!groupByName[name]) {
        groupByName[name] = { name:name, windows:[] };
        groups.push(groupByName[name]);
      }
      groupByName[name].windows.push(item);
    });
    var shell = element("div", "quota-groups");
    groups.forEach(function (group) {
      group.windows.sort(function (left, right) {
        var rank = quotaWindowRank(left) - quotaWindowRank(right);
        return rank || translateWindowLabel(left.label).localeCompare(translateWindowLabel(right.label), "zh-CN");
      });
      var section = element("section", "quota-group");
      var heading = element("div", "quota-group-head");
      heading.appendChild(element("h3", "quota-group-title", redactSecrets(translateDisplayText(group.name))));
      heading.appendChild(element("span", "quota-group-count", group.windows.length + " 个周期"));
      section.appendChild(heading);
      var grid = element("div", "quota-window-grid");
      group.windows.forEach(function (item) { grid.appendChild(quotaWindowCard(item)); });
      section.appendChild(grid);
      shell.appendChild(section);
    });
    card.appendChild(shell);
    return true;
  }

  function refreshCountdowns() {
    document.querySelectorAll("[data-reset-at]").forEach(function (node) {
      var resetAt = finiteNumber(node.getAttribute("data-reset-at"));
      if (resetAt == null) return;
      var seconds = Math.max(0, (resetAt - Date.now()) / 1000);
      node.textContent = seconds > 0 ? durationText(seconds) + "后重置" : "即将重置";
    });
  }

  function renderAccountMeta(card, result) {
    var details = element("div", "account-meta");
    if (result.plan) {
      var plan = element("span", "detail");
      plan.appendChild(element("span", "detail-label", "套餐"));
      plan.appendChild(element("span", "", redactSecrets(translateDisplayText(result.plan))));
      details.appendChild(plan);
    }
    if (result.reset_at) {
      var reset = element("span", "detail");
      reset.appendChild(element("span", "detail-label", "重置"));
      reset.appendChild(element("span", "", formatDateTime(result.reset_at)));
      details.appendChild(reset);
    }
    if (details.childNodes.length) card.appendChild(details);
  }

  function extraDetailKeys(result) {
    if (!result.extra || typeof result.extra !== "object") return [];
    return Object.keys(result.extra).sort(function (left, right) {
      var labelOrder = detailLabel(left).localeCompare(detailLabel(right), "zh-CN");
      return labelOrder || left.localeCompare(right, "zh-CN");
    });
  }

  function renderExtraDetails(card, result, keys) {
    keys = Array.isArray(keys) ? keys : extraDetailKeys(result);
    if (!keys.length) return false;
    var section = element("section", "detail-section");
    section.appendChild(element("h3", "detail-section-title", "账户明细"));
    var list = element("dl", "detail-grid");
    keys.forEach(function (key) {
      var row = element("div", "detail-row");
      row.appendChild(element("dt", "", detailLabel(key)));
      row.appendChild(element("dd", "", redactSecrets(result.extra[key])));
      list.appendChild(row);
    });
    section.appendChild(list);
    card.appendChild(section);
    return true;
  }

  function resultCard(result, index) {
    var consoleOnly = Boolean(result.error) && /控制台|官网登录|订阅管理页/.test(String(result.error));
    var failed = Boolean(result.error) && !consoleOnly;
    var detailKeys = !result.error ? extraDetailKeys(result) : [];
    var detailsID = "account-details-" + index + "-" + tinyHash(String(result.account_name || "") + "|" + String(result.base_url || ""));
    var card = element("article", "result-card" + (failed ? " error" : consoleOnly ? " limited" : ""));
    card.style.animationDelay = Math.min(index * 35, 210) + "ms";
    var head = element("div", "result-head");
    var identity = element("div", "provider-cell");
    identity.appendChild(element("div", "result-name", result.account_name || result.provider || "余额账户"));
    identity.appendChild(element("div", "result-url", result.base_url || ""));
    head.appendChild(identity);
    var badge = element("span", "badge " + (failed ? "failure" : consoleOnly ? "warning" : "success"));
    badge.appendChild(icon(failed || consoleOnly ? "alert" : "check"));
    var hasWindows = Array.isArray(result.quota_windows) && result.quota_windows.length > 0;
    badge.appendChild(element("span", "", failed ? "查询失败" : consoleOnly ? "仅控制台可查" : hasWindows ? "配额已更新" : "查询成功"));
    var actions = element("div", "result-actions");
    actions.appendChild(badge);
    var detailButton = null;
    if (detailKeys.length) {
      detailButton = element("button", "detail-toggle");
      detailButton.type = "button";
      detailButton.setAttribute("aria-expanded", "false");
      detailButton.setAttribute("aria-controls", detailsID);
      detailButton.setAttribute("aria-label", "查看“" + (result.account_name || result.provider || "余额账户") + "”的账户明细");
      detailButton.appendChild(element("span", "detail-toggle-label", "查看账户明细"));
      detailButton.appendChild(icon("chevron"));
      actions.appendChild(detailButton);
    }
    head.appendChild(actions);
    card.appendChild(head);

    var overview = element("div", "result-overview");
    if (failed || consoleOnly) {
      overview.appendChild(element("div", "quota-main failure", consoleOnly ? "当前模型密钥不能直接查询余额" : "查询失败，请检查密钥、接口地址或账户状态"));
      overview.appendChild(element("div", "error-detail", localizedError(result.error)));
      card.appendChild(overview);
    } else {
      overview.appendChild(element("div", "quota-main", formatBalance(result)));
      renderAccountMeta(overview, result);
      card.appendChild(overview);
      var renderedWindows = renderQuotaGroups(card, result);
      var total = Number(result.tokens_total || 0);
      var used = Number(result.tokens_used || 0);
      if (!renderedWindows && total > 0) {
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
      if (detailKeys.length) {
        var collapse = element("div", "account-detail-collapse");
        collapse.id = detailsID;
        collapse.setAttribute("aria-hidden", "true");
        collapse.setAttribute("inert", "");
        var detailInner = element("div", "account-detail-inner");
        renderExtraDetails(detailInner, result, detailKeys);
        collapse.appendChild(detailInner);
        card.appendChild(collapse);
        detailButton.addEventListener("click", function () {
          var expanded = detailButton.getAttribute("aria-expanded") === "true";
          var nextExpanded = !expanded;
          detailButton.setAttribute("aria-expanded", String(nextExpanded));
          detailButton.setAttribute("aria-label", (nextExpanded ? "收起“" : "查看“") + (result.account_name || result.provider || "余额账户") + "”的账户明细");
          collapse.setAttribute("aria-hidden", String(!nextExpanded));
          if (nextExpanded) collapse.removeAttribute("inert");
          else collapse.setAttribute("inert", "");
          card.classList.toggle("details-open", nextExpanded);
          setText(detailButton.querySelector(".detail-toggle-label"), nextExpanded ? "收起账户明细" : "查看账户明细");
        });
      }
    }

    var foot = element("div", "result-foot");
    foot.appendChild(element("span", "key-preview", result.key_preview || "密钥已隐藏"));
    foot.appendChild(element("span", "", formatTime(result.fetched_at) ? "更新于 " + formatTime(result.fetched_at) : "刚刚更新"));
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
    refreshCountdowns();
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

  function updateQueryHelp(target, value) {
    target.textContent = "";
    var definition = providerDefinitions[value];
    if (!definition || definition.status !== "console_only") {
      target.hidden = true;
      return;
    }
    target.hidden = false;
    target.appendChild(icon("alert"));
    target.appendChild(element("span", "", definition.description || "该平台仅能在官网登录控制台查看余额。"));
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
        option.textContent = definition.label + (definition.status === "console_only" ? "（仅控制台可查）" : "");
        select.appendChild(option);
      });
      select.value = state.draftMappings[provider.mappingKey] || "";
      var queryHelp = element("div", "query-help");
      updateQueryHelp(queryHelp, select.value);
      select.addEventListener("change", function () {
        if (select.value) state.draftMappings[provider.mappingKey] = select.value;
        else delete state.draftMappings[provider.mappingKey];
        updateQueryHelp(queryHelp, select.value);
        state.dirty = true;
        setText(byID("save-state"), "有未保存的更改");
      });
      queryCell.appendChild(select);
      queryCell.appendChild(queryHelp);
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
  window.setInterval(refreshCountdowns, 30000);

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
