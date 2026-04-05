// godom Injector — background service worker
// Handles extension icon click and godom.js injection requests.

// Click extension icon → open options in a new tab
chrome.action.onClicked.addListener(() => {
  chrome.tabs.create({ url: chrome.runtime.getURL("options/options.html") });
});

chrome.runtime.onMessage.addListener((msg, sender, sendResponse) => {
  if (msg.type === "INJECT") {
    const tabId = sender.tab?.id;
    if (!tabId) {
      sendResponse({ error: "No tab" });
      return;
    }
    injectGodom(tabId, msg.appUrl, msg.scriptPath, msg.wsUrl, msg.allowRoot).then(
      () => sendResponse({ ok: true }),
      (err) => sendResponse({ error: err.message })
    );
    return true;
  }
});

async function injectGodom(tabId, appUrl, scriptPath, wsUrl, allowRoot) {
  // Fetch the godom.js bundle from the app server.
  // The service worker can fetch from any origin (LAN IPs, etc.)
  // regardless of the target page's CSP.
  const scriptUrl = appUrl.replace(/\/$/, "") + (scriptPath || "/godom.js");
  const resp = await fetch(scriptUrl);
  if (!resp.ok) throw new Error(`Failed to fetch ${scriptUrl}: ${resp.status}`);
  let bundleCode = await resp.text();

  const fullCode = `
    window.GODOM_WS_URL = ${JSON.stringify(wsUrl)};
    window.GODOM_INJECT_ALLOW_ROOT = ${allowRoot ? "true" : "false"};
    ${bundleCode}
  `;

  // Inject into the page's main world via chrome.scripting.executeScript.
  // This API bypasses the page's CSP entirely — the func runs as
  // extension-privileged code in the page's JS context.
  // We use a blob URL to load the code as a script, which also
  // bypasses CSP script-src restrictions.
  await chrome.scripting.executeScript({
    target: { tabId, frameIds: [0] },
    world: "MAIN",
    func: (code, iconUrl) => {
      if (window.__GODOM_INJECTED__) return;
      window.__GODOM_INJECTED__ = true;
      const blob = new Blob([code], { type: "application/javascript" });
      const url = URL.createObjectURL(blob);
      const script = document.createElement("script");
      script.src = url;
      script.onload = () => URL.revokeObjectURL(url);
      document.head.appendChild(script);

      // Floating indicator (only in embedded mode)
      if (!iconUrl) return;
      const badge = document.createElement("div");
      badge.innerHTML = `<img src="${iconUrl}" width="20" height="20" style="display:block">`;
      badge.title = "godom active";
      badge.style.cssText = "position:fixed;bottom:12px;right:12px;z-index:2147483647;background:#0B1120;border-radius:8px;padding:6px;cursor:pointer;opacity:0.7;transition:opacity 0.2s;box-shadow:0 2px 8px rgba(0,0,0,0.3);";
      badge.onmouseenter = () => badge.style.opacity = "1";
      badge.onmouseleave = () => badge.style.opacity = "0.7";
      document.body.appendChild(badge);
    },
    args: [fullCode, allowRoot ? null : chrome.runtime.getURL("icons/icon48.png")],
  });
}
