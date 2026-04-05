// godom Injector — content script
// Checks current page URL against saved rules and triggers injection.

(async function () {
  // Only run in the top frame, not iframes
  if (window !== window.top) return;

  const url = window.location.href;

  const result = await chrome.storage.local.get("godom_rules");
  const rules = result.godom_rules || [];

  for (const rule of rules) {
    if (!rule.enabled) continue;
    if (!rule.appUrl) continue;

    // Check excludes first — excludes win over includes
    if (matchesAny(url, rule.exclude || [])) continue;

    // Check includes
    if (!matchesAny(url, rule.include || [])) continue;

    // Build URLs
    const appUrl = rule.appUrl.replace(/\/$/, "");
    const scriptPath = rule.scriptPath || "/godom.js";
    const wsUrl = appUrl.replace(/^http/, "ws") + "/ws";

    // Ask background to fetch and inject godom.js.
    chrome.runtime.sendMessage({
      type: "INJECT",
      appUrl: appUrl,
      scriptPath: scriptPath,
      wsUrl: wsUrl,
      allowRoot: rule.allowRoot || false,
    }, (resp) => {
      if (resp && resp.error) {
        console.error("[godom] Injection failed:", resp.error);
      }
    });

    // Only inject one rule per page
    break;
  }
})();

// Match URL against an array of patterns.
// Patterns support * (any chars) and ? (single char).
// Example: "https://github.com/*/pulls" matches "https://github.com/foo/pulls"
function matchesAny(url, patterns) {
  for (const pattern of patterns) {
    const p = pattern.trim();
    if (!p) continue;
    const regex = patternToRegex(p);
    if (regex && regex.test(url)) return true;
  }
  return false;
}

function patternToRegex(pattern) {
  try {
    const escaped = pattern
      .replace(/[.+^${}()|[\]\\]/g, "\\$&")
      .replace(/\*/g, ".*")
      .replace(/\?/g, ".");
    return new RegExp("^" + escaped + "$");
  } catch {
    return null;
  }
}
