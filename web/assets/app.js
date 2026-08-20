(() => {
  const state = {
    apiKey: "",
    status: null,
    instances: [],
    lastSyncPayload: null,
    lastSettingsJSON: "",
    safeMode: true,
    syncBusy: false,
    libOffset: 0,
    libTotal: 0,
    settingsHydrating: false,
    autosaveTimer: 0,
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
    for (const id of ["syncSourceInstance", "syncTargetInstance", "libIngestInstance"]) {
      const el = $(id);
      if (!el) continue;
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
    $("stPlex").textContent = plexOverviewLabel(st);
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

  function plexOverviewLabel(st) {
    if (st.plexAccountUsername) return `linked as ${st.plexAccountUsername}`;
    if (st.plexConfigured) return "configured";
    return "not set";
  }

  function setSafeMode(on) {
    state.safeMode = Boolean(on);
    $("btnApply").disabled = state.safeMode || !state.lastSyncPayload || state.syncBusy;
    if (state.syncBusy) {
      $("applyHint").textContent = "Sync in progress…";
      return;
    }
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
          <input type="checkbox" class="arr-advanced-check" />
          <span>Advanced</span>
        </label>
        <div class="arr-advanced-body" hidden>
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
    // Keep cookie value in the DOM even when Advanced is collapsed; only show the pane when checked.
    if (authCookie) {
      row.querySelector(".arr-advanced-check").checked = true;
      row.querySelector(".arr-advanced-body").hidden = false;
    }
    row.querySelector(".arr-remove").addEventListener("click", () => {
      row.remove();
      scheduleAutosave();
    });
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

  function renderPlexStatus(plex = {}) {
    const linked = Boolean(plex.token);
    const who = plex.accountUsername ? ` as ${plex.accountUsername}` : "";
    $("plexStatus").textContent = linked ? `Linked${who}` : "Not linked";
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
    const plex = set.plex || {};
    $("setPlexURL").value = plex.serverUrl || "";
    $("setPlexToken").value = plex.token || "";
    $("setPlexToken").type = "password";
    const plexToggle = document.querySelector('[data-toggle-secret="setPlexToken"]');
    if (plexToggle) plexToggle.textContent = "Show";
    renderPlexStatus(plex);
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
    let incomplete = false;
    for (const row of $("arrInstances").querySelectorAll(".arr-row")) {
      const name = row.querySelector(".arr-name").value.trim();
      const url = row.querySelector(".arr-url").value.trim();
      const apiKey = row.querySelector(".arr-key").value.trim();
      const kind = row.querySelector(".arr-kind").value;
      const authCookie = row.querySelector(".arr-cookie").value.trim();
      if (!name && !url && !apiKey && !authCookie) continue;
      if (!name || !url || !apiKey) {
        incomplete = true;
        continue;
      }
      arrInstances.push({ name, kind, url, apiKey, authCookie });
    }
    return {
      incomplete,
      apiKey: $("setApiKey").value.trim(),
      instanceName: $("setInstanceName").value.trim(),
      safeMode: $("setSafeMode").checked,
      torboxSearchPerHour: Number($("setTorbox").value) || 60,
      tmdbApiKey: $("setTmdb").value.trim(),
      plex: {
        serverUrl: $("setPlexURL").value.trim(),
        token: $("setPlexToken").value.trim(),
      },
      arrInstances,
    };
  }

  function settingsPayload(collected) {
    return {
      apiKey: collected.apiKey,
      instanceName: collected.instanceName,
      safeMode: collected.safeMode,
      torboxSearchPerHour: collected.torboxSearchPerHour,
      tmdbApiKey: collected.tmdbApiKey,
      plex: collected.plex,
      arrInstances: collected.arrInstances,
    };
  }

  function setSettingsHint(msg) {
    $("settingsHint").textContent = msg;
  }

  function scheduleAutosave() {
    if (state.settingsHydrating) return;
    window.clearTimeout(state.autosaveTimer);
    state.autosaveTimer = window.setTimeout(() => {
      persistSettings().catch((err) => setSettingsHint(err.message));
    }, 400);
  }

  async function persistSettings() {
    if (state.settingsHydrating) return;
    const collected = collectSettings();
    if (!collected.apiKey || !collected.instanceName) {
      setSettingsHint("Instance name and API key are required.");
      return;
    }
    if (collected.incomplete) {
      setSettingsHint("Finish the instance row (name, URL, API key) to save.");
      return;
    }
    const payload = settingsPayload(collected);
    const snap = JSON.stringify(payload);
    if (snap === state.lastSettingsJSON) return;
    setSettingsHint("Saving…");
    const saved = await api("/api/v1/settings", {
      method: "PUT",
      body: snap,
    });
    if (saved.apiKey) {
      state.apiKey = saved.apiKey;
    }
    state.lastSettingsJSON = snap;
    await refresh();
    setSettingsHint("Saved");
  }

  async function loadSettings() {
    state.settingsHydrating = true;
    try {
      const set = await api("/api/v1/settings");
      renderSettings(set);
      state.lastSettingsJSON = JSON.stringify(settingsPayload(collectSettings()));
      setSettingsHint("Changes save automatically.");
    } finally {
      state.settingsHydrating = false;
    }
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
    const isCatalog = src === "listarr-go";
    $("wrapSourceInstance").hidden = !isArr;
    $("wrapTmdbIds").hidden = isArr || isCatalog;
    $("wrapCatalogWatched").hidden = !isCatalog;
    $("wrapCatalogUnwatched").hidden = !isCatalog;
    $("syncMonitored").closest("label").hidden = isCatalog;
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
        rootFolderPath: selectedRootFolderPath(),
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
    } else if (source === "listarr-go") {
      payload.catalogFilter = {
        watchedOnly: $("syncCatalogWatchedOnly").checked,
        unwatchedOnly: $("syncCatalogUnwatchedOnly").checked,
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

  function setSyncBusy(on, message) {
    state.syncBusy = Boolean(on);
    $("syncBusy").hidden = !on;
    if (message) $("syncBusyText").textContent = message;
    $("btnPreview").disabled = on;
    setSafeMode(state.safeMode);
  }

  async function runSync(apply) {
    const payload = buildSyncPayload();
    const count = payload.maxItems || 100;
    const label = apply
      ? `Applying up to ${count} titles — lookup and add can take a few minutes…`
      : `Building preview of up to ${count} titles…`;
    setSyncBusy(true, label);
    $("syncMeta").hidden = false;
    $("syncMeta").textContent = apply ? "Apply in progress…" : "Preview in progress…";
    try {
      const path = apply ? "/api/v1/sync/apply" : "/api/v1/sync/preview";
      const res = await api(path, { method: "POST", body: JSON.stringify(payload) });
      state.lastSyncPayload = payload;
      renderSyncResult(res);
      toast(apply ? "Apply finished" : "Preview ready");
    } finally {
      setSyncBusy(false);
    }
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

  const CUSTOM_ROOT = "__custom__";

  function fillQualityProfiles(profiles) {
    const el = $("syncQP");
    const prev = Number(el.value) || 0;
    el.innerHTML = "";
    const list = profiles || [];
    if (!list.length) {
      const opt = document.createElement("option");
      opt.value = "1";
      opt.textContent = "1";
      el.appendChild(opt);
      return;
    }
    for (const profile of list) {
      const opt = document.createElement("option");
      opt.value = String(profile.id);
      opt.textContent = profile.name ? `${profile.name} (${profile.id})` : String(profile.id);
      el.appendChild(opt);
    }
    const ids = list.map((p) => p.id);
    el.value = ids.includes(prev) ? String(prev) : String(list[0].id);
  }

  function selectedRootFolderPath() {
    if ($("syncRoot").value === CUSTOM_ROOT) {
      return $("syncRootCustom").value.trim();
    }
    return ($("syncRoot").value || "").trim();
  }

  function setCustomRootVisible(on) {
    $("syncRootCustom").hidden = !on;
    $("syncRootCustom").required = on;
  }

  function appendRootGroup(select, label, items) {
    if (!items.length) return;
    const group = document.createElement("optgroup");
    group.label = label;
    for (const item of items) {
      const opt = document.createElement("option");
      opt.value = item.value;
      opt.textContent = item.text;
      group.appendChild(opt);
    }
    select.appendChild(group);
  }

  function uniqueRootItems(rows, seen, textFn) {
    const items = [];
    for (const row of rows || []) {
      const path = String((row && row.path) || "").trim();
      if (!path || seen.has(path)) continue;
      seen.add(path);
      items.push({ value: path, text: textFn(row, path) });
    }
    return items;
  }

  function fillRootFolders(arrRoots, plexLibs) {
    const el = $("syncRoot");
    const prev = selectedRootFolderPath();
    const seen = new Set();
    const arrItems = uniqueRootItems(arrRoots, seen, (_row, path) => path);
    const plexItems = uniqueRootItems(plexLibs, seen, (lib, path) => {
      const title = String((lib && lib.sectionTitle) || "").trim();
      return title ? `${title} (${path})` : path;
    });
    el.innerHTML = "";
    appendRootGroup(el, "Target *arr", arrItems);
    appendRootGroup(el, "Plex libraries", plexItems);
    const custom = document.createElement("option");
    custom.value = CUSTOM_ROOT;
    custom.textContent = "Custom path…";
    el.appendChild(custom);
    applyRootSelection(el, prev, [...arrItems, ...plexItems].map((i) => i.value));
  }

  function applyRootSelection(select, prev, values) {
    if (!values.length) {
      select.value = CUSTOM_ROOT;
      if (prev) $("syncRootCustom").value = prev;
      setCustomRootVisible(true);
      return;
    }
    if (prev && values.includes(prev)) {
      select.value = prev;
      setCustomRootVisible(false);
      return;
    }
    if (prev) {
      select.value = CUSTOM_ROOT;
      $("syncRootCustom").value = prev;
      setCustomRootVisible(true);
      return;
    }
    select.value = values[0];
    setCustomRootVisible(false);
  }

  function rootFolderHint(arrCount, plexCount, arrErr, plexErr) {
    const parts = [];
    if (arrCount) parts.push(`${arrCount} *arr root(s)`);
    if (plexCount) parts.push(`${plexCount} Plex path(s)`);
    if (parts.length) {
      let msg = `${parts.join(", ")} — pick a path or Custom.`;
      if (arrErr) msg += ` *arr extras unavailable (${arrErr}).`;
      if (plexErr) msg += ` Plex extras unavailable (${plexErr}).`;
      return msg;
    }
    if (arrErr && plexErr) {
      return `*arr (${arrErr}); Plex (${plexErr}). Choose Custom path.`;
    }
    if (arrErr) return `Target *arr options unavailable (${arrErr}). Choose Custom path.`;
    if (plexErr) return `Plex libraries unavailable (${plexErr}). Choose Custom path.`;
    return "No *arr or Plex paths — choose Custom path and type one.";
  }

  async function refreshTargetOptions() {
    const name = $("syncTargetInstance").value;
    let arrRoots = [];
    let plexLibs = [];
    let arrErr = "";
    let plexErr = "";
    if (name) {
      try {
        const body = await api(`/api/v1/arr/${encodeURIComponent(name)}/options`);
        arrRoots = body.rootFolders || [];
        fillQualityProfiles(body.qualityProfiles || []);
      } catch (err) {
        fillQualityProfiles([]);
        arrErr = err.message;
      }
    } else {
      fillQualityProfiles([]);
    }
    const mediaType = $("syncMedia").value === "tv" ? "tv" : "movie";
    try {
      const body = await api(`/api/v1/plex/libraries?mediaType=${encodeURIComponent(mediaType)}`);
      plexLibs = body.libraries || [];
    } catch (err) {
      plexErr = err.message;
    }
    fillRootFolders(arrRoots, plexLibs);
    $("syncRootHint").textContent = rootFolderHint(arrRoots.length, plexLibs.length, arrErr, plexErr);
  }

  async function linkPlex() {
    const pin = await api("/api/v1/plex/auth/pin", { method: "POST", body: "{}" });
    if (pin.authUrl) {
      window.open(pin.authUrl, "_blank", "noopener,noreferrer");
    }
    toast(`Approve code ${pin.code} in Plex, then return here`);
    const deadline = Date.now() + 5 * 60 * 1000;
    while (Date.now() < deadline) {
      await new Promise((r) => setTimeout(r, 2000));
      const status = await api(`/api/v1/plex/auth/pin/${pin.id}`);
      if (status.linked) {
        await loadSettings();
        await refresh();
        await refreshTargetOptions();
        toast(status.accountUsername ? `Linked as ${status.accountUsername}` : "Plex linked");
        return;
      }
    }
    throw new Error("Plex link timed out — try again");
  }

  async function testPlex() {
    const body = await api("/api/v1/plex/test", {
      method: "POST",
      body: JSON.stringify({
        serverUrl: $("setPlexURL").value.trim(),
        token: $("setPlexToken").value.trim(),
      }),
    });
    if (!body.ok) throw new Error(body.message || "plex test failed");
    toast(body.serverName ? `OK — ${body.serverName}` : "OK — Plex connected");
  }

  async function unlinkPlex() {
    if (!window.confirm("Unlink Plex auth token from settings?")) return;
    await api("/api/v1/plex/auth", { method: "DELETE" });
    $("setPlexToken").value = "";
    renderPlexStatus({});
    await refresh();
    toast("Plex unlinked");
  }

  const libPageSize = 200;

  async function loadLibrary(resetOffset) {
    if (resetOffset) state.libOffset = 0;
    const media = $("libMedia").value;
    const watched = $("libWatched").value;
    const q = encodeURIComponent($("libQuery").value.trim());
    let path = `/api/v1/catalog/titles?limit=${libPageSize}&offset=${state.libOffset}`;
    if (media) path += `&mediaType=${encodeURIComponent(media)}`;
    if (watched !== "all") path += `&watched=${encodeURIComponent(watched)}`;
    if (q) path += `&q=${q}`;
    const body = await api(path);
    const wrap = $("libraryWrap");
    const tbody = $("libraryResults").querySelector("tbody");
    tbody.innerHTML = "";
    wrap.hidden = false;
    const total = body.total ?? 0;
    state.libTotal = total;
    const start = total === 0 ? 0 : state.libOffset + 1;
    const end = Math.min(state.libOffset + (body.titles || []).length, total);
    $("libMeta").textContent = `${total} titles in catalog · showing ${start}–${end} · backend ${body.backend || "—"}`;
    const pager = $("libPager");
    pager.hidden = total <= libPageSize;
    $("libPageLabel").textContent = `${start}–${end} of ${total}`;
    $("btnLibPrev").disabled = state.libOffset <= 0;
    $("btnLibNext").disabled = state.libOffset + libPageSize >= total;
    for (const title of body.titles || []) {
      const tr = document.createElement("tr");
      const seasons = Array.isArray(title.seasons) ? title.seasons.length : 0;
      const sources = (title.sourceInstances || []).join(", ");
      const collection = title.collectionName || (title.collectionTmdbId ? String(title.collectionTmdbId) : "");
      tr.innerHTML = `<td>${escapeHtml(title.title || "")}</td><td>${escapeHtml(title.mediaType || "")}</td><td>${title.year || ""}</td><td>${title.tmdbId || ""}</td><td>${escapeHtml(title.imdbId || "")}</td><td>${escapeHtml(collection)}</td><td>${seasons || ""}</td><td>${title.watched ? "yes" : "no"}</td><td>${escapeHtml(sources)}</td>`;
      tbody.appendChild(tr);
    }
  }

  async function ingestLibrary() {
    const payload = {
      sourceInstance: $("libIngestInstance").value,
      mediaType: $("libIngestMedia").value,
      sourceFilter: { monitoredOnly: false },
    };
    $("btnLibIngest").disabled = true;
    $("libMeta").textContent = "Ingesting full *arr library…";
    try {
      const res = await api("/api/v1/catalog/ingest", {
        method: "POST",
        body: JSON.stringify(payload),
      });
      const avail = res.available ?? res.fetched ?? 0;
      const extra = res.truncated ? ` (truncated at ${res.fetched})` : "";
      toast(`Ingested ${res.fetched ?? 0} of ${avail}${extra} · added ${res.upsert?.added ?? 0} · updated ${res.upsert?.updated ?? 0}`);
      await loadLibrary(true);
    } finally {
      $("btnLibIngest").disabled = false;
    }
  }

  async function refreshPlexWatched() {
    const media = $("libMedia").value;
    let path = "/api/v1/catalog/plex-watched";
    if (media) path += `?mediaType=${encodeURIComponent(media)}`;
    const res = await api(path, { method: "POST", body: "{}" });
    toast(`Plex watched · fetched ${res.fetched ?? 0} · updated ${res.updated ?? 0}`);
    await loadLibrary();
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
    if (name === "sync" && apiKey()) {
      refreshTargetOptions().catch(() => {});
    }
    if (name === "library" && apiKey()) {
      loadLibrary().catch((err) => toast(err.message));
    }
  }

  function wire() {
    toggleSourceFields();
    for (const btn of document.querySelectorAll(".tab")) {
      btn.addEventListener("click", () => switchTab(btn.dataset.tab));
    }
    $("syncSource").addEventListener("change", toggleSourceFields);
    $("syncSourceAdvanced").addEventListener("change", (ev) => {
      $("syncSourceAdvancedBody").hidden = !ev.currentTarget.checked;
    });
    $("syncTargetAdvanced").addEventListener("change", (ev) => {
      $("syncTargetAdvancedBody").hidden = !ev.currentTarget.checked;
    });
    $("syncMedia").addEventListener("change", () => {
      refreshTargetOptions().catch(() => {});
    });
    $("syncTargetInstance").addEventListener("change", () => {
      refreshTargetOptions().catch(() => {});
    });
    $("syncRoot").addEventListener("change", () => {
      const custom = $("syncRoot").value === CUSTOM_ROOT;
      setCustomRootVisible(custom);
      if (custom) $("syncRootCustom").focus();
    });
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
    $("libraryForm").addEventListener("submit", (ev) => {
      ev.preventDefault();
      loadLibrary(true).catch((err) => toast(err.message));
    });
    $("btnLibPrev").addEventListener("click", () => {
      state.libOffset = Math.max(0, state.libOffset - libPageSize);
      loadLibrary().catch((err) => toast(err.message));
    });
    $("btnLibNext").addEventListener("click", () => {
      state.libOffset += libPageSize;
      loadLibrary().catch((err) => toast(err.message));
    });
    $("btnLibIngest").addEventListener("click", () => {
      ingestLibrary().catch((err) => toast(err.message));
    });
    $("btnLibPlexWatched").addEventListener("click", () => {
      refreshPlexWatched().catch((err) => toast(err.message));
    });
    $("btnAddArr").addEventListener("click", () => {
      $("arrInstances").appendChild(arrRow());
    });
    $("settingsForm").addEventListener("input", scheduleAutosave);
    $("settingsForm").addEventListener("change", () => {
      persistSettings().catch((err) => setSettingsHint(err.message));
    });
    $("settingsForm").addEventListener("submit", (ev) => {
      ev.preventDefault();
      persistSettings().catch((err) => setSettingsHint(err.message));
    });
    $("btnPlexLink").addEventListener("click", () => {
      linkPlex().catch((err) => toast(err.message));
    });
    $("btnPlexTest").addEventListener("click", () => {
      testPlex().catch((err) => toast(err.message));
    });
    $("btnPlexUnlink").addEventListener("click", () => {
      unlinkPlex().catch((err) => toast(err.message));
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
      if (!window.confirm("Generate a new API key? It replaces the current key immediately.")) {
        return;
      }
      $("setApiKey").value = generateApiKeyHex();
      $("setApiKey").type = "password";
      const toggle = document.querySelector('[data-toggle-secret="setApiKey"]');
      if (toggle) toggle.textContent = "Show";
      persistSettings()
        .then(() => toast("New API key saved"))
        .catch((err) => setSettingsHint(err.message));
    });
    bootstrap().catch((err) => {
      $("chipInstance").textContent = "unavailable";
      $("chipHint").textContent = err.message;
      toast(err.message);
    });
  }

  wire();
})();
