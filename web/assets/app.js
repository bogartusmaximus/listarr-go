(() => {
  const state = {
    apiKey: "",
    status: null,
    instances: [],
    lastSyncPayload: null,
    safeMode: true,
  };

  const $ = (id) => document.getElementById(id);

  function toast(msg) {
    const el = $("toast");
    el.textContent = msg;
    el.hidden = false;
    window.clearTimeout(toast._t);
    toast._t = window.setTimeout(() => {
      el.hidden = true;
    }, 4200);
  }

  function apiKey() {
    return state.apiKey || "";
  }

  async function api(path, opts = {}) {
    const headers = Object.assign({}, opts.headers || {});
    const key = apiKey();
    if (!key) throw new Error("UI bootstrap has not loaded an API key yet");
    headers["X-Api-Key"] = key;
    if (opts.body && !headers["Content-Type"]) {
      headers["Content-Type"] = "application/json";
    }
    const res = await fetch(path, Object.assign({}, opts, { headers }));
    const text = await res.text();
    let body = null;
    if (text) {
      try {
        body = JSON.parse(text);
      } catch {
        body = { raw: text };
      }
    }
    if (!res.ok) {
      const msg = (body && (body.message || body.description)) || res.statusText;
      throw new Error(msg);
    }
    return body;
  }

  function parseIntList(raw) {
    return String(raw || "")
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean)
      .map((s) => Number(s))
      .filter((n) => Number.isInteger(n) && n > 0);
  }

  function fillInstanceSelects() {
    const names = state.instances.map((i) => i.name);
    for (const id of ["syncSourceInstance", "syncTargetInstance"]) {
      const el = $(id);
      const prev = el.value;
      el.innerHTML = "";
      if (!names.length) {
        const opt = document.createElement("option");
        opt.value = "";
        opt.textContent = "(no instances configured)";
        el.appendChild(opt);
        continue;
      }
      for (const name of names) {
        const opt = document.createElement("option");
        opt.value = name;
        opt.textContent = name;
        el.appendChild(opt);
      }
      if (names.includes(prev)) el.value = prev;
    }
    if (names.includes("local")) $("syncSourceInstance").value = "local";
    if (names.includes("remote")) $("syncTargetInstance").value = "remote";
    else if (names.includes("radarr")) $("syncTargetInstance").value = "radarr";
  }

  function renderOverview() {
    const st = state.status || {};
    $("stConnection").textContent = state.status ? "connected" : "not connected";
    $("stVersion").textContent = st.version || "—";
    $("stSafeMode").textContent = st.safeMode ? "on (read-only)" : "off (writes allowed)";
    $("stBudget").textContent =
      st.torboxSearchPerHour != null
        ? `${st.torboxSearchRemaining ?? "?"} / ${st.torboxSearchPerHour}`
        : "—";
    $("stStore").textContent = st.storeBackend || "—";
    $("stTmdb").textContent = st.tmdbConfigured ? "configured" : "not set";
    $("footVersion").textContent = `listarr-go ${st.version || ""}`.trim();

    const ul = $("instanceList");
    ul.innerHTML = "";
    if (!state.instances.length) {
      const li = document.createElement("li");
      li.textContent = "No named instances yet — configure them in Settings.";
      ul.appendChild(li);
      return;
    }
    for (const inst of state.instances) {
      const li = document.createElement("li");
      li.innerHTML = `<strong>${inst.name}</strong><span>${inst.kind}</span>`;
      ul.appendChild(li);
    }
  }

  function setSafeMode(on) {
    state.safeMode = Boolean(on);
    $("btnApply").disabled = state.safeMode || !state.lastSyncPayload;
    $("applyHint").textContent = state.safeMode
      ? "Apply stays locked while Safe Mode is on."
      : "Safe Mode is off — Apply will write to the target *arr. Preview first.";
  }

  function arrRow(inst = {}) {
    const row = document.createElement("div");
    row.className = "arr-row";
    const name = escapeHtml(inst.name || "");
    const url = escapeHtml(inst.url || "");
    const apiKey = escapeHtml(inst.apiKey || "");
    const authCookie = escapeHtml(inst.authCookie || "");
    const kind = inst.kind === "sonarr" ? "sonarr" : "radarr";
    const advancedOpen = Boolean(inst.authCookie);
    row.innerHTML = `
      <label class="field"><span>Name</span><input type="text" class="arr-name" value="${name}" placeholder="local" required /></label>
      <label class="field"><span>Kind</span>
        <select class="arr-kind">
          <option value="radarr"${kind === "radarr" ? " selected" : ""}>radarr</option>
          <option value="sonarr"${kind === "sonarr" ? " selected" : ""}>sonarr</option>
        </select>
      </label>
      <label class="field"><span>URL</span><input type="url" class="arr-url" value="${url}" placeholder="http://127.0.0.1:7878" required /></label>
      <label class="field secret-field"><span>API key</span>
        <div class="secret-row">
          <input type="password" class="arr-key" value="${apiKey}" autocomplete="off" spellcheck="false" required />
          <button type="button" class="btn arr-toggle">Show</button>
          <button type="button" class="btn arr-copy">Copy</button>
        </div>
      </label>
      <button type="button" class="btn arr-test">Test</button>
      <button type="button" class="btn danger arr-remove">Remove</button>
      <div class="arr-advanced">
        <label class="field check arr-advanced-toggle">
          <input type="checkbox" class="arr-advanced-check"${advancedOpen ? " checked" : ""} />
          <span>Advanced</span>
        </label>
        <div class="arr-advanced-body"${advancedOpen ? "" : " hidden"}>
          <label class="field secret-field"><span>Auth cookie</span>
            <div class="secret-row">
              <input type="password" class="arr-cookie" value="${authCookie}" autocomplete="off" spellcheck="false" placeholder="_oauth2_proxy=…" />
              <button type="button" class="btn arr-cookie-toggle">Show</button>
              <button type="button" class="btn arr-cookie-copy">Copy</button>
            </div>
          </label>
          <p class="hint">Sent as Cookie header for reverse-proxy auth (e.g. oauth2-proxy).</p>
        </div>
      </div>
    `;
    row.querySelector(".arr-remove").addEventListener("click", () => row.remove());
    row.querySelector(".arr-test").addEventListener("click", () => {
      testArrRow(row).catch((err) => toast(err.message));
    });
    row.querySelector(".arr-advanced-check").addEventListener("change", (ev) => {
      row.querySelector(".arr-advanced-body").hidden = !ev.currentTarget.checked;
    });
    row.querySelector(".arr-toggle").addEventListener("click", (ev) => {
      const input = row.querySelector(".arr-key");
      const show = input.type === "password";
      input.type = show ? "text" : "password";
      ev.currentTarget.textContent = show ? "Hide" : "Show";
    });
    row.querySelector(".arr-copy").addEventListener("click", async () => {
      const input = row.querySelector(".arr-key");
      try {
        await navigator.clipboard.writeText(input.value || "");
        toast("Copied");
      } catch {
        toast("Copy failed");
      }
    });
    row.querySelector(".arr-cookie-toggle").addEventListener("click", (ev) => {
      const input = row.querySelector(".arr-cookie");
      const show = input.type === "password";
      input.type = show ? "text" : "password";
      ev.currentTarget.textContent = show ? "Hide" : "Show";
    });
    row.querySelector(".arr-cookie-copy").addEventListener("click", async () => {
      const input = row.querySelector(".arr-cookie");
      try {
        await navigator.clipboard.writeText(input.value || "");
        toast("Copied");
      } catch {
        toast("Copy failed");
      }
    });
    return row;
  }

  async function testArrRow(row) {
    const payload = {
      name: row.querySelector(".arr-name").value.trim(),
      kind: row.querySelector(".arr-kind").value,
      url: row.querySelector(".arr-url").value.trim(),
      apiKey: row.querySelector(".arr-key").value.trim(),
      authCookie: row.querySelector(".arr-cookie").value.trim(),
    };
    if (!payload.url || !payload.apiKey) {
      throw new Error("URL and API key are required to test");
    }
    const res = await fetch("/api/v1/arr/test", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Api-Key": apiKey(),
      },
      body: JSON.stringify(payload),
    });
    const text = await res.text();
    let body = null;
    if (text) {
      try {
        body = JSON.parse(text);
      } catch {
        body = { message: text };
      }
    }
    if (!res.ok || !body || !body.ok) {
      const msg = (body && (body.message || body.description)) || res.statusText;
      throw new Error(msg || "connection test failed");
    }
    const label = [body.appName, body.version].filter(Boolean).join(" ");
    toast(label ? `OK — ${label}` : "OK — connected");
  }

  function generateApiKeyHex() {
    const bytes = new Uint8Array(16);
    crypto.getRandomValues(bytes);
    return Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
  }

  function renderSettings(set) {
    $("setInstanceName").value = set.instanceName || "";
    $("setApiKey").value = set.apiKey || "";
    $("setApiKey").type = "password";
    const toggle = document.querySelector('[data-toggle-secret="setApiKey"]');
    if (toggle) toggle.textContent = "Show";
    $("setSafeMode").checked = set.safeMode !== false;
    $("setTorbox").value = set.torboxSearchPerHour || 60;
    $("setTmdb").value = set.tmdbApiKey || "";
    $("setTmdb").type = "password";
    const tmdbToggle = document.querySelector('[data-toggle-secret="setTmdb"]');
    if (tmdbToggle) tmdbToggle.textContent = "Show";
    const wrap = $("arrInstances");
    wrap.innerHTML = "";
    const list = set.arrInstances || [];
    if (!list.length) {
      wrap.appendChild(arrRow());
    } else {
      for (const inst of list) wrap.appendChild(arrRow(inst));
    }
  }

  function collectSettings() {
    const arrInstances = [];
    for (const row of $("arrInstances").querySelectorAll(".arr-row")) {
      const name = row.querySelector(".arr-name").value.trim();
      const url = row.querySelector(".arr-url").value.trim();
      const apiKey = row.querySelector(".arr-key").value.trim();
      const kind = row.querySelector(".arr-kind").value;
      const authCookie = row.querySelector(".arr-cookie").value.trim();
      if (!name && !url && !apiKey) continue;
      arrInstances.push({ name, kind, url, apiKey, authCookie });
    }
    return {
      apiKey: $("setApiKey").value.trim(),
      instanceName: $("setInstanceName").value.trim(),
      safeMode: $("setSafeMode").checked,
      torboxSearchPerHour: Number($("setTorbox").value) || 60,
      tmdbApiKey: $("setTmdb").value.trim(),
      arrInstances,
    };
  }

  async function loadSettings() {
    const set = await api("/api/v1/settings");
    renderSettings(set);
  }

  async function saveSettings(ev) {
    ev.preventDefault();
    const payload = collectSettings();
    const saved = await api("/api/v1/settings", {
      method: "PUT",
      body: JSON.stringify(payload),
    });
    if (saved.apiKey) {
      state.apiKey = saved.apiKey;
    }
    renderSettings(saved);
    await refresh();
    toast("Settings saved");
  }

  async function refresh() {
    state.status = await api("/api/v1/system/status");
    const body = await api("/api/v1/arr/instances");
    state.instances = body.instances || [];
    fillInstanceSelects();
    renderOverview();
    setSafeMode(state.status.safeMode !== false);
    $("chipInstance").textContent = state.status.instanceName || "listarr";
    $("chipHint").textContent = "Ready — API key is in Settings.";
  }

  async function bootstrap() {
    const res = await fetch("/api/v1/ui/bootstrap");
    const text = await res.text();
    let body = null;
    if (text) {
      try {
        body = JSON.parse(text);
      } catch {
        body = null;
      }
    }
    if (!res.ok || !body || !body.apiKey) {
      const msg = (body && body.message) || res.statusText || "bootstrap failed";
      throw new Error(msg);
    }
    state.apiKey = body.apiKey;
    $("chipInstance").textContent = body.instanceName || "listarr";
    await refresh();
  }

  function toggleSourceFields() {
    const src = $("syncSource").value;
    const isArr = src === "arr-library";
    $("wrapSourceInstance").hidden = !isArr;
    $("wrapTmdbIds").hidden = isArr;
  }

  function buildSyncPayload() {
    const source = $("syncSource").value;
    const mediaType = $("syncMedia").value;
    const payload = {
      source,
      mediaType,
      maxItems: Number($("syncMax").value) || 100,
      target: {
        instance: $("syncTargetInstance").value,
        rootFolderPath: $("syncRoot").value.trim(),
        qualityProfileId: Number($("syncQP").value),
        tags: parseIntList($("syncTargetTags").value),
        monitored: $("syncTargetMonitored").checked,
        searchOnAdd: $("syncSearchOnAdd").checked,
        seasonFolder: $("syncSeasonFolder").checked,
      },
    };
    if (source === "arr-library") {
      payload.sourceInstance = $("syncSourceInstance").value;
      payload.sourceFilter = {
        monitoredOnly: $("syncMonitored").checked,
        tagIds: parseIntList($("syncTagIds").value),
        pathContains: $("syncPath").value.trim(),
      };
    } else {
      payload.tmdbIds = parseIntList($("syncTmdbIds").value);
    }
    return payload;
  }

  function renderSyncResult(res) {
    state.lastSyncPayload = buildSyncPayload();
    setSafeMode(state.safeMode);
    const meta = $("syncMeta");
    meta.hidden = false;
    meta.textContent = `${res.dryRun ? "Preview" : "Apply"} · adds ${res.adds} · skips ${res.skips} · deferred ${res.deferredSearch || 0} · errors ${res.errors}`;
    const wrap = $("syncResultsWrap");
    const tbody = $("syncResults").querySelector("tbody");
    tbody.innerHTML = "";
    wrap.hidden = false;
    for (const item of res.items || []) {
      const tr = document.createElement("tr");
      const cls = `action-${item.action || "skip"}`;
      tr.innerHTML = `<td>${escapeHtml(item.title || "")}</td><td>${item.tmdbId ?? ""}</td><td class="${cls}">${escapeHtml(item.action || "")}</td><td>${escapeHtml(item.detail || "")}</td>`;
      tbody.appendChild(tr);
    }
  }

  function escapeHtml(s) {
    return String(s)
      .replaceAll("&", "&amp;")
      .replaceAll("<", "&lt;")
      .replaceAll(">", "&gt;")
      .replaceAll('"', "&quot;");
  }

  async function runSync(apply) {
    const payload = buildSyncPayload();
    const path = apply ? "/api/v1/sync/apply" : "/api/v1/sync/preview";
    const res = await api(path, { method: "POST", body: JSON.stringify(payload) });
    state.lastSyncPayload = payload;
    renderSyncResult(res);
    toast(apply ? "Apply finished" : "Preview ready");
  }

  async function runDiscover(ev) {
    ev.preventDefault();
    const media = $("discoverMedia").value;
    const sort = encodeURIComponent($("discoverSort").value.trim() || "popularity.desc");
    const page = Number($("discoverPage").value) || 1;
    const path =
      media === "tv"
        ? `/api/v1/discover/tv?sort_by=${sort}&page=${page}`
        : `/api/v1/discover/movies?sort_by=${sort}&page=${page}`;
    const body = await api(path);
    const wrap = $("discoverWrap");
    const tbody = $("discoverResults").querySelector("tbody");
    tbody.innerHTML = "";
    wrap.hidden = false;
    for (const item of body.items || []) {
      const tr = document.createElement("tr");
      tr.innerHTML = `<td>${escapeHtml(item.title || "")}</td><td>${item.tmdbId ?? ""}</td><td>${item.year ?? ""}</td>`;
      tbody.appendChild(tr);
    }
  }

  async function loadActivity() {
    const body = await api("/api/v1/activity?limit=50");
    const wrap = $("activityWrap");
    const tbody = $("activityResults").querySelector("tbody");
    tbody.innerHTML = "";
    wrap.hidden = false;
    for (const run of body.runs || []) {
      const tr = document.createElement("tr");
      const when = run.createdAt ? new Date(run.createdAt).toLocaleString() : "";
      const route = `${run.sourceInstance || "—"} → ${run.targetInstance || "—"}`;
      tr.innerHTML = `<td>${escapeHtml(when)}</td><td>${run.dryRun ? "preview" : "apply"}</td><td>${escapeHtml(run.source || "")}</td><td>${escapeHtml(run.mediaType || "")}</td><td>${escapeHtml(route)}</td><td>${run.adds ?? 0}</td><td>${run.skips ?? 0}</td><td>${run.errors ?? 0}</td>`;
      tbody.appendChild(tr);
    }
  }

  function switchTab(name) {
    for (const btn of document.querySelectorAll(".tab")) {
      const on = btn.dataset.tab === name;
      btn.classList.toggle("active", on);
      btn.setAttribute("aria-selected", on ? "true" : "false");
    }
    for (const panel of document.querySelectorAll(".panel")) {
      const on = panel.dataset.panel === name;
      panel.hidden = !on;
      panel.classList.toggle("active", on);
    }
    if (name === "activity" && apiKey()) {
      loadActivity().catch((err) => toast(err.message));
    }
    if (name === "settings" && apiKey()) {
      loadSettings().catch((err) => toast(err.message));
    }
  }

  function wire() {
    toggleSourceFields();
    for (const btn of document.querySelectorAll(".tab")) {
      btn.addEventListener("click", () => switchTab(btn.dataset.tab));
    }
    $("syncSource").addEventListener("change", toggleSourceFields);
    $("syncForm").addEventListener("submit", (ev) => {
      ev.preventDefault();
      runSync(false).catch((err) => toast(err.message));
    });
    $("btnApply").addEventListener("click", () => {
      if (!window.confirm("Apply will mutate the target *arr. Continue?")) return;
      runSync(true).catch((err) => toast(err.message));
    });
    $("discoverForm").addEventListener("submit", (ev) => {
      runDiscover(ev).catch((err) => toast(err.message));
    });
    $("btnRefreshActivity").addEventListener("click", () => {
      loadActivity().catch((err) => toast(err.message));
    });
    $("btnAddArr").addEventListener("click", () => {
      $("arrInstances").appendChild(arrRow());
    });
    $("settingsForm").addEventListener("submit", (ev) => {
      saveSettings(ev).catch((err) => toast(err.message));
    });
    for (const btn of document.querySelectorAll("[data-toggle-secret]")) {
      btn.addEventListener("click", () => {
        const id = btn.getAttribute("data-toggle-secret");
        const input = $(id);
        const show = input.type === "password";
        input.type = show ? "text" : "password";
        btn.textContent = show ? "Hide" : "Show";
      });
    }
    for (const btn of document.querySelectorAll("[data-copy-secret]")) {
      btn.addEventListener("click", async () => {
        const id = btn.getAttribute("data-copy-secret");
        try {
          await navigator.clipboard.writeText($(id).value || "");
          toast("Copied");
        } catch {
          toast("Copy failed");
        }
      });
    }
    $("btnRegenApiKey").addEventListener("click", () => {
      if (!window.confirm("Generate a new API key? Save settings to apply it.")) {
        return;
      }
      $("setApiKey").value = generateApiKeyHex();
      $("setApiKey").type = "password";
      const toggle = document.querySelector('[data-toggle-secret="setApiKey"]');
      if (toggle) toggle.textContent = "Show";
      toast("New API key generated — save to apply");
    });
    bootstrap().catch((err) => {
      $("chipInstance").textContent = "unavailable";
      $("chipHint").textContent = err.message;
      toast(err.message);
    });
  }

  wire();
})();
