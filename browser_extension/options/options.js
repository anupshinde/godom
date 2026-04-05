const STORAGE_KEY = "godom_rules";

const rulesList = document.getElementById("rules-list");
const emptyState = document.getElementById("empty-state");
const addRuleBtn = document.getElementById("add-rule-btn");
const editorOverlay = document.getElementById("editor-overlay");
const editorTitle = document.getElementById("editor-title");
const edName = document.getElementById("ed-name");
const edAppUrl = document.getElementById("ed-app-url");
const edScriptPath = document.getElementById("ed-script-path");
const edUrlWarning = document.getElementById("ed-url-warning");
const edAllowRoot = document.getElementById("ed-allow-root");
const edInclude = document.getElementById("ed-include");
const edExclude = document.getElementById("ed-exclude");
const edSave = document.getElementById("ed-save");
const edCancel = document.getElementById("ed-cancel");

let rules = [];
let editingIndex = -1; // -1 = new rule

// --- Storage ---

async function loadRules() {
  const result = await chrome.storage.local.get(STORAGE_KEY);
  rules = result[STORAGE_KEY] || [];
  renderRules();
}

async function saveRules() {
  await chrome.storage.local.set({ [STORAGE_KEY]: rules });
  renderRules();
}

// --- Render rule list ---

function renderRules() {
  rulesList.innerHTML = "";
  emptyState.hidden = rules.length > 0;

  for (let i = 0; i < rules.length; i++) {
    const rule = rules[i];
    const card = document.createElement("div");
    card.className = "rule-card" + (rule.enabled ? "" : " disabled");

    const includeCount = (rule.include || []).filter((p) => p.trim()).length;
    const excludeCount = (rule.exclude || []).filter((p) => p.trim()).length;

    card.innerHTML = `
      <div class="rule-toggle">
        <input type="checkbox" ${rule.enabled ? "checked" : ""} title="Enable/disable">
      </div>
      <div class="rule-info">
        <div class="rule-name">${escapeHtml(rule.name || "Unnamed")}</div>
        <div class="rule-url">${escapeHtml(rule.appUrl || "\u2014")}</div>
        <div class="rule-patterns">${includeCount} include, ${excludeCount} exclude</div>
      </div>
      <div class="rule-actions">
        <button class="edit">Edit</button>
        <button class="delete">Del</button>
      </div>
    `;

    // Toggle
    card.querySelector(".rule-toggle input").addEventListener("change", (e) => {
      rules[i].enabled = e.target.checked;
      saveRules();
    });

    // Edit
    card.querySelector(".edit").addEventListener("click", () => openEditor(i));

    // Delete
    card.querySelector(".delete").addEventListener("click", () => {
      rules.splice(i, 1);
      saveRules();
    });

    rulesList.appendChild(card);
  }
}

// --- Editor ---

function openEditor(index) {
  editingIndex = index;
  const rule = index >= 0 ? rules[index] : {};

  editorTitle.textContent = index >= 0 ? "Edit Rule" : "New Rule";
  edName.value = rule.name || "";
  edAppUrl.value = rule.appUrl || "";
  edScriptPath.value = rule.scriptPath || "";
  edAllowRoot.checked = rule.allowRoot || false;
  edInclude.value = (rule.include || []).join("\n");
  edExclude.value = (rule.exclude || []).join("\n");

  checkUrlWarning();
  editorOverlay.hidden = false;
  edName.focus();
}

function checkUrlWarning() {
  const url = edAppUrl.value.trim();
  if (url && url.startsWith("http://") && !url.includes("localhost") && !url.includes("127.0.0.1")) {
    edUrlWarning.textContent = "Non-HTTPS URL with a remote host. Injection into HTTPS pages will fail due to mixed content blocking. Use HTTPS (e.g. via Caddy reverse proxy) or localhost.";
    edUrlWarning.hidden = false;
  } else {
    edUrlWarning.hidden = true;
  }
}

edAppUrl.addEventListener("input", checkUrlWarning);

function closeEditor() {
  editorOverlay.hidden = true;
  editingIndex = -1;
}

function collectEditor() {
  const include = edInclude.value.split("\n").filter((l) => l.trim());
  const exclude = edExclude.value.split("\n").filter((l) => l.trim());

  return {
    name: edName.value.trim() || "Unnamed",
    appUrl: edAppUrl.value.trim(),
    scriptPath: edScriptPath.value.trim() || "",
    allowRoot: edAllowRoot.checked,
    include,
    exclude,
    enabled: editingIndex >= 0 ? rules[editingIndex].enabled : true,
  };
}

// --- Events ---

addRuleBtn.addEventListener("click", () => openEditor(-1));

edCancel.addEventListener("click", closeEditor);
document.getElementById("ed-close").addEventListener("click", closeEditor);

edSave.addEventListener("click", () => {
  const rule = collectEditor();
  if (!rule.appUrl) {
    edAppUrl.focus();
    return;
  }
  if (rule.include.length === 0) {
    edInclude.focus();
    return;
  }

  if (editingIndex >= 0) {
    rules[editingIndex] = rule;
  } else {
    rules.push(rule);
  }
  saveRules();
  closeEditor();
});

// Close overlay on background click
editorOverlay.addEventListener("click", (e) => {
  if (e.target === editorOverlay) closeEditor();
});

function escapeHtml(str) {
  const div = document.createElement("div");
  div.textContent = str;
  return div.innerHTML;
}

// --- Export / Import ---

const importOverlay = document.getElementById("import-overlay");
const importJson = document.getElementById("import-json");

function closeImportDialog() {
  importOverlay.hidden = true;
  importJson.value = "";
}

const importCopy = document.getElementById("import-copy");

// Export: show JSON in dialog with copy button
document.getElementById("export-btn").addEventListener("click", () => {
  importJson.value = JSON.stringify(rules, null, 2);
  importJson.readOnly = true;
  importOverlay.hidden = false;
  document.querySelector("#import-overlay h2").textContent = "Export Rules";
  document.querySelector("#import-overlay .import-hint").textContent = "Copy the JSON below:";
  document.getElementById("import-save").hidden = true;
  importCopy.hidden = false;
  importJson.focus();
  importJson.select();
});

// Import: paste JSON
document.getElementById("import-btn").addEventListener("click", () => {
  importJson.value = "";
  importJson.readOnly = false;
  importOverlay.hidden = false;
  document.querySelector("#import-overlay h2").textContent = "Import Rules";
  document.querySelector("#import-overlay .import-hint").textContent = "Paste your exported JSON below:";
  document.getElementById("import-save").hidden = false;
  importCopy.hidden = true;
  importJson.focus();
});

importCopy.addEventListener("click", async () => {
  await navigator.clipboard.writeText(importJson.value);
  const orig = importCopy.textContent;
  importCopy.textContent = "Copied!";
  setTimeout(() => { importCopy.textContent = orig; }, 1500);
});

document.getElementById("import-save").addEventListener("click", async () => {
  try {
    const imported = JSON.parse(importJson.value);
    if (!Array.isArray(imported)) throw new Error("Expected an array of rules");
    rules = imported;
    await saveRules();
    closeImportDialog();
  } catch (err) {
    alert("Invalid JSON: " + err.message);
  }
});

document.getElementById("import-cancel").addEventListener("click", closeImportDialog);
document.getElementById("import-close").addEventListener("click", closeImportDialog);
importOverlay.addEventListener("click", (e) => {
  if (e.target === importOverlay) closeImportDialog();
});

// Init
loadRules();
