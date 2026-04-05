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

      // Sidebar panel (shadow DOM for CSS isolation)
      const sidebarHost = document.createElement("div");
      sidebarHost.style.cssText = "position:fixed;top:0;right:0;bottom:0;width:0;z-index:2147483647;transition:width 0.25s ease;";
      document.body.appendChild(sidebarHost);
      const shadow = sidebarHost.attachShadow({ mode: "open" });
      shadow.innerHTML = `
        <style>
          :host { all: initial; }
          .panel {
            width: 320px; height: 100%;
            background: #fff;
            box-shadow: -2px 0 12px rgba(0,0,0,0.15);
            transform: translateX(100%);
            transition: transform 0.25s ease;
            display: flex; flex-direction: column;
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
            font-size: 14px; color: #1a1a1a;
          }
          .panel.open { transform: translateX(0); }
          .header {
            display: flex; align-items: center; justify-content: space-between;
            padding: 12px 16px;
            border-bottom: 1px solid #e5e5e5;
            background: #0B1120; color: #E8F4FD;
          }
          .header h2 { font-size: 14px; font-weight: 600; margin: 0; }
          .close-btn {
            background: none; border: none; color: #aac;
            font-size: 20px; cursor: pointer; padding: 0 4px;
          }
          .close-btn:hover { color: #fff; }
          .content { flex: 1; overflow-y: auto; padding: 16px; }
          .status { color: #888; font-size: 13px; }
        </style>
        <div class="panel">
          <div class="header">
            <h2>godom</h2>
            <button class="close-btn">&times;</button>
          </div>
          <div class="content">
            <p class="status">godom is active on this page.</p>
          </div>
        </div>
      `;
      const panel = shadow.querySelector(".panel");

      let sidebarOpen = false;
      function toggle() {
        sidebarOpen = !sidebarOpen;
        panel.classList.toggle("open", sidebarOpen);
        sidebarHost.style.width = sidebarOpen ? "320px" : "0";
      }

      badge.addEventListener("click", toggle);
      shadow.querySelector(".close-btn").addEventListener("click", toggle);
    },
    args: [fullCode, allowRoot ? null : chrome.runtime.getURL("icons/icon48.png")],
  });
}
