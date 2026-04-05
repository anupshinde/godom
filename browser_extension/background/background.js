// godom Injector — background service worker
// Handles extension icon click and godom.js injection requests.

// Click extension icon → open options in a new tab
chrome.action.onClicked.addListener(() => {
  chrome.tabs.create({ url: chrome.runtime.getURL("options/options.html") });
});

chrome.runtime.onMessage.addListener((msg, sender, sendResponse) => {
  if (msg.type === "HIDE_RULE") {
    chrome.storage.local.get("godom_rules", (result) => {
      const rules = result.godom_rules || [];
      if (msg.ruleIndex >= 0 && msg.ruleIndex < rules.length) {
        rules[msg.ruleIndex].hidden = true;
        chrome.storage.local.set({ godom_rules: rules }, () => {
          sendResponse({ ok: true });
        });
      } else {
        sendResponse({ error: "Invalid rule index" });
      }
    });
    return true;
  }

  if (msg.type === "INJECT") {
    const tabId = sender.tab?.id;
    if (!tabId) {
      sendResponse({ error: "No tab" });
      return;
    }
    injectGodom(tabId, msg.appUrl, msg.scriptPath, msg.wsUrl, msg.allowRoot, msg.panelComponent, msg.panelIsolateCSS, msg.hidden, msg.ruleIndex).then(
      () => sendResponse({ ok: true }),
      (err) => sendResponse({ error: err.message })
    );
    return true;
  }
});

async function injectGodom(tabId, appUrl, scriptPath, wsUrl, allowRoot, panelComponent, panelIsolateCSS, hidden, ruleIndex) {
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
    func: (code, iconUrl, icon16Url, panelComponent, panelIsolateCSS, hidden, ruleIndex) => {
      if (window.__GODOM_INJECTED__) return;
      window.__GODOM_INJECTED__ = true;
      const blob = new Blob([code], { type: "application/javascript" });
      const url = URL.createObjectURL(blob);
      const script = document.createElement("script");
      script.src = url;
      script.onload = () => URL.revokeObjectURL(url);
      document.head.appendChild(script);

      // Floating indicator (only in embedded mode, skip if hidden)
      if (!iconUrl || hidden) return;
      const badge = document.createElement("div");
      badge.innerHTML = `<img src="${iconUrl}" width="20" height="20" style="display:block">`;
      badge.title = "godom active";
      badge.style.cssText = "position:fixed;bottom:12px;right:12px;z-index:2147483647;background:#0B1120;border-radius:8px;padding:6px;cursor:pointer;opacity:0.7;transition:opacity 0.2s;box-shadow:0 0 0 2px rgba(255,255,255,0.6),0 2px 8px rgba(0,0,0,0.3);";
      badge.onmouseenter = () => badge.style.opacity = "1";
      badge.onmouseleave = () => badge.style.opacity = "0.7";
      document.body.appendChild(badge);

      // Sidebar panel (inline styles, no shadow DOM)
      const style = document.createElement("style");
      style.textContent = `
        .__godom-panel { position:fixed;top:0;right:0;bottom:0;width:0;z-index:2147483647;transition:width 0.25s ease,left 0.25s ease;overflow:hidden;background:#fff;box-shadow:-2px 0 12px rgba(0,0,0,0.15);display:flex;flex-direction:column;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;font-size:14px;color:#1a1a1a; }
        .__godom-panel.maximized { left:0;width:100% !important;box-shadow:none; }
        .__godom-panel.maximized .__godom-panel-header { flex-direction:row-reverse; }
        .__godom-panel.maximized .__godom-panel-header img { display:none !important; }
        .__godom-panel-handle { position:absolute;top:0;left:-4px;bottom:0;width:8px;cursor:col-resize;background:transparent;z-index:1; }
        .__godom-panel-handle:hover { background:rgba(0,0,0,0.1); }
        .__godom-panel-header { display:flex;align-items:center;justify-content:space-between;padding:12px 16px;border-bottom:1px solid #e5e5e5;background:#0B1120;color:#E8F4FD;flex-shrink:0; }
        .__godom-panel-close { background:none;border:none;color:#aac;font-size:20px;cursor:pointer;padding:0 4px; }
        .__godom-panel-close:hover { color:#fff; }
        .__godom-panel-menu-wrap { position:relative; }
        .__godom-panel-menu-btn { background:none;border:none;color:#aac;font-size:18px;cursor:pointer;padding:0 4px;line-height:1; }
        .__godom-panel-menu-btn:hover { color:#fff; }
        .__godom-panel-menu { display:none;position:absolute;top:100%;right:0;margin-top:4px;background:#1a2234;border:1px solid #2a3a5a;border-radius:6px;min-width:140px;box-shadow:0 4px 12px rgba(0,0,0,0.3);z-index:10;overflow:hidden; }
        .__godom-panel-menu.open { display:block; }
        .__godom-panel-menu button { display:block;width:100%;background:none;border:none;color:#ccd;font-size:13px;padding:8px 14px;text-align:left;cursor:pointer; }
        .__godom-panel-menu button:hover { background:#2a3a5a;color:#fff; }
        .__godom-panel-content { flex:1;overflow-y:auto;padding:16px; }
      `;
      document.head.appendChild(style);

      const panel = document.createElement("div");
      panel.className = "__godom-panel";
      panel.innerHTML = '<div class="__godom-panel-handle"></div>'
        + '<div class="__godom-panel-header">'
        + '<span style="display:flex;align-items:center;gap:8px">'
        + '<button class="__godom-panel-close">&times;</button>'
        + '<img src="' + icon16Url + '" width="16" height="16" style="display:block">'
        + '</span>'
        + '<div class="__godom-panel-menu-wrap">'
        + '<button class="__godom-panel-menu-btn" title="Menu">&#x22EE;</button>'
        + '<div class="__godom-panel-menu">'
        + '<button class="__godom-menu-close">Close Panel</button>'
        + '<button class="__godom-menu-maximize">Maximize</button>'
        + '<button class="__godom-menu-hide">Hide Badge</button>'
        + '</div>'
        + '</div>'
        + '</div>'
        + '<div class="__godom-panel-content" g-component="' + panelComponent + '"' + (panelIsolateCSS ? ' g-shadow' : '') + '></div>';
      document.body.appendChild(panel);

      const handle = panel.querySelector(".__godom-panel-handle");
      const menuWrap = panel.querySelector(".__godom-panel-menu-wrap");

      // Restore panel state from sessionStorage
      var saved = {};
      try { saved = JSON.parse(sessionStorage.__godom_panel || "{}"); } catch(e) {}
      let sidebarOpen = false;
      let sidebarWidth = saved.width || 320;

      function saveState() {
        sessionStorage.__godom_panel = JSON.stringify({ open: sidebarOpen, width: sidebarWidth });
      }
      function toggle() {
        sidebarOpen = !sidebarOpen;
        panel.style.width = sidebarOpen ? sidebarWidth + "px" : "0";
        document.body.style.marginRight = sidebarOpen ? sidebarWidth + "px" : "";
        saveState();
      }

      let maximized = false;

      // Auto-open if it was open on previous page
      if (saved.open) { toggle(); }

      badge.addEventListener("click", toggle);
      panel.querySelector(".__godom-panel-close").addEventListener("click", () => {
        if (maximized) {
          maximized = false;
          panel.classList.remove("maximized");
          handle.style.display = "";
          menuWrap.style.display = "";
          document.body.style.marginRight = sidebarOpen ? sidebarWidth + "px" : "";
          badge.style.display = "";
          maxBtn.textContent = "Maximize";
        } else {
          toggle();
        }
      });

      // Kebab menu
      const menuBtn = panel.querySelector(".__godom-panel-menu-btn");
      const menu = panel.querySelector(".__godom-panel-menu");
      menuBtn.addEventListener("click", (e) => {
        e.stopPropagation();
        menu.classList.toggle("open");
      });
      document.addEventListener("click", () => menu.classList.remove("open"));

      // Menu: Close Panel
      panel.querySelector(".__godom-menu-close").addEventListener("click", () => {
        menu.classList.remove("open");
        if (maximized) {
          maximized = false;
          panel.classList.remove("maximized");
          handle.style.display = "";
          menuWrap.style.display = "";
          badge.style.display = "";
          maxBtn.textContent = "Maximize";
        }
        if (sidebarOpen) toggle();
      });

      // Menu: Maximize/Restore — toggle full overlay
      const maxBtn = panel.querySelector(".__godom-menu-maximize");
      maxBtn.addEventListener("click", () => {
        menu.classList.remove("open");
        maximized = !maximized;
        panel.classList.toggle("maximized", maximized);
        handle.style.display = maximized ? "none" : "";
        menuWrap.style.display = maximized ? "none" : "";
        document.body.style.marginRight = maximized ? "0" : (sidebarOpen ? sidebarWidth + "px" : "");
        badge.style.display = maximized ? "none" : "";
        maxBtn.textContent = maximized ? "Restore" : "Maximize";
      });

      // Menu: Hide Badge
      panel.querySelector(".__godom-menu-hide").addEventListener("click", () => {
        menu.classList.remove("open");
        badge.remove();
        panel.remove();
        document.body.style.marginRight = "";
        delete sessionStorage.__godom_panel;
        document.dispatchEvent(new CustomEvent("__godom_hide_rule", { detail: { ruleIndex } }));
      });

      // Resize by dragging left edge
      handle.addEventListener("mousedown", (e) => {
        e.preventDefault();
        const startX = e.clientX;
        const startW = sidebarWidth;
        panel.style.transition = "none";
        function onMove(e) {
          const delta = startX - e.clientX;
          sidebarWidth = Math.max(200, Math.min(startW + delta, window.innerWidth * 0.8));
          panel.style.width = sidebarWidth + "px";
          document.body.style.marginRight = sidebarWidth + "px";
        }
        function onUp() {
          document.removeEventListener("mousemove", onMove);
          document.removeEventListener("mouseup", onUp);
          panel.style.transition = "";
          saveState();
        }
        document.addEventListener("mousemove", onMove);
        document.addEventListener("mouseup", onUp);
      });
    },
    args: [fullCode, allowRoot ? null : chrome.runtime.getURL("icons/icon48.png"), chrome.runtime.getURL("icons/icon16.png"), panelComponent || "extension", panelIsolateCSS !== false, hidden, ruleIndex],
  });
}
