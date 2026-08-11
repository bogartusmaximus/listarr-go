(() => {
  const KEY = "listarrApiKey";
  const state = {
    status: null,
    instances: [],
    lastSyncPayload: null,
    applyEnabled: false,
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
    return ($("apiKey").value || "").trim();
  }

  function saveKey() {
    const k = apiKey();
    if (k) sessionStorage.setItem(KEY, k);
    else sessionStorage.removeItem(KEY);
  }

  function loadKey() {
    const k = sessionStorage.getItem(KEY) || "";
    $("apiKey").value = k;
  }

  async function api(path, opts = {}) {
    const headers = Object.assign({}, opts.headers || {});
    const key = apiKey();
    if (!key) throw new Error("Enter an API key and connect first");
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
    $("stApply").textContent = st.applyEnabled ? "enabled" : "disabled";
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
      li.textContent = "No named instances yet — set LISTARR_ARR_* env vars.";
      ul.appendChild(li);
      return;
    }
    for (const inst of state.instances) {
      const li = document.createElement("li");
      li.innerHTML = `<strong>${inst.name}</strong><span>${inst.kind}</span>`;
      ul.appendChild(li);
    }
  }

  function setApplyEnabled(on) {
    state.applyEnabled = Boolean(on);
    $("btnApply").disabled = !state.applyEnabled || !state.lastSyncPayload;
    $("applyHint").textContent = state.applyEnabled
      ? "Apply will mutate the target *arr. Preview first."
      : "Apply stays locked until the server has LISTARR_APPLY=1.";
  }

  async function connect() {
    saveKey();
    state.status = await api("/api/v1/system/status");
    const body = await api("/api/v1/arr/instances");
    state.instances = body.instances || [];
    fillInstanceSelects();
    renderOverview();
    setApplyEnabled(state.status.applyEnabled);
    $("authHint").textContent = `Connected to ${state.status.instanceName || "listarr"}.`;
    toast("Connected");
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
    setApplyEnabled(state.applyEnabled);
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
  }

  function wire() {
    loadKey();
    toggleSourceFields();
    $("btnConnect").addEventListener("click", () => {
      connect().catch((err) => toast(err.message));
    });
    $("apiKey").addEventListener("keydown", (ev) => {
      if (ev.key === "Enter") {
        ev.preventDefault();
        connect().catch((err) => toast(err.message));
      }
    });
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
    if (apiKey()) {
      connect().catch(() => {
        $("authHint").textContent = "Saved key present — click Connect.";
      });
    }
  }

  wire();
})();
