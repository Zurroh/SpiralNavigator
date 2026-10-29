"use strict";

/* ---------- small helpers ---------- */
const $ = (s, el = document) => el.querySelector(s);
const esc = (t) => String(t).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));

async function api(path, body) {
  const opts = body === undefined ? {} : { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) };
  let res;
  try { res = await fetch(path, opts); } catch { throw new Error("The launcher lost its connection to itself. Close it and open it again."); }
  const data = await res.json().catch(() => ({}));
  if (!res.ok) { const e = new Error(data.error || "Something went wrong."); e.data = data; throw e; }
  return data;
}

let toastTimer;
function toast(msg) {
  const t = $("#toast");
  t.textContent = msg;
  t.classList.add("show");
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => t.classList.remove("show"), 3200);
}

function ago(unix) {
  if (!unix) return "";
  const s = Math.max(0, Math.floor(Date.now() / 1000 - unix));
  if (s < 45) return "just now";
  if (s < 3600) { const m = Math.round(s / 60); return m + (m === 1 ? " minute ago" : " minutes ago"); }
  if (s < 86400) { const h = Math.round(s / 3600); return h + (h === 1 ? " hour ago" : " hours ago"); }
  const d = Math.round(s / 86400); return d + (d === 1 ? " day ago" : " days ago");
}

function bytes(n) {
  if (n < 1024) return n + " B";
  if (n < 1048576) return (n / 1024).toFixed(0) + " KB";
  if (n < 1073741824) return (n / 1048576).toFixed(1) + " MB";
  return (n / 1073741824).toFixed(2) + " GB";
}

const SCHOOLS = ["fire", "ice", "storm", "myth", "life", "death", "balance"];

/* ---------- markdown (escapes everything first; only http/https links) ---------- */
function mdInline(src) {
  const codes = [];
  let t = esc(src).replace(/`([^`]+)`/g, (m, c) => { codes.push(c); return "\u0000" + (codes.length - 1) + "\u0000"; });
  t = t.replace(/!\[([^\]]*)\]\((https?:\/\/[^\s)]+)\)/g, (m, alt, u) => `<img src="${u}" alt="${alt}" loading="lazy">`);
  t = t.replace(/\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)/g, (m, txt, u) => `<a href="${u}" data-ext="1">${txt}</a>`);
  t = t.replace(/(^|[\s(])(https?:\/\/(?:(?!&quot;|&#39;)[^\s<)])*[^\s<).,;:!?&])/g, (m, pre, u) => `${pre}<a href="${u}" data-ext="1">${u}</a>`);
  t = t.replace(/\*\*([^*]+?)\*\*|__([^_]+?)__/g, (m, a, b) => `<strong>${a || b}</strong>`);
  t = t.replace(/(^|[^*\w])\*([^*\s](?:[^*]*[^*\s])?)\*(?!\*)/g, (m, pre, a) => `${pre}<em>${a}</em>`);
  t = t.replace(/(^|[^_\w])_([^_\s](?:[^_]*[^_\s])?)_(?![_\w])/g, (m, pre, a) => `${pre}<em>${a}</em>`);
  t = t.replace(/~~([^~]+)~~/g, "<del>$1</del>");
  t = t.replace(/\u0000(\d+)\u0000/g, (m, i) => `<code>${codes[+i]}</code>`);
  return t;
}

function mdRender(src) {
  const lines = src.replace(/\r\n?/g, "\n").split("\n");
  const out = [];
  let i = 0;
  const isBlank = (l) => /^\s*$/.test(l);
  const listRe = /^(\s*)([-*+]|\d+[.)])\s+(.*)$/;
  const tableSep = /^\s*\|?\s*:?-{2,}:?\s*(\|\s*:?-{2,}:?\s*)*\|?\s*$/;

  while (i < lines.length) {
    const line = lines[i];
    if (isBlank(line)) { i++; continue; }

    let m;
    if ((m = line.match(/^\s*(`{3,}|~{3,})\s*(\S*)\s*$/))) {
      const fence = m[1]; const buf = []; i++;
      while (i < lines.length && !lines[i].trim().startsWith(fence)) buf.push(lines[i++]);
      i++;
      out.push(`<pre><code>${esc(buf.join("\n"))}</code></pre>`);
      continue;
    }
    if ((m = line.match(/^\s{0,3}(#{1,4})\s+(.*?)\s*#*\s*$/))) {
      out.push(`<h${m[1].length}>${mdInline(m[2])}</h${m[1].length}>`); i++; continue;
    }
    if (/^\s{0,3}([-*_])(\s*\1){2,}\s*$/.test(line)) { out.push("<hr>"); i++; continue; }
    if (/^\s{0,3}>/.test(line)) {
      const buf = [];
      while (i < lines.length && /^\s{0,3}>/.test(lines[i])) buf.push(lines[i++].replace(/^\s{0,3}>\s?/, ""));
      out.push(`<blockquote>${mdRender(buf.join("\n"))}</blockquote>`);
      continue;
    }
    if (line.includes("|") && i + 1 < lines.length && tableSep.test(lines[i + 1]) && lines[i + 1].includes("-")) {
      const cells = (l) => l.trim().replace(/^\||\|$/g, "").split("|").map((c) => c.trim());
      const head = cells(line); i += 2;
      const rows = [];
      while (i < lines.length && lines[i].includes("|") && !isBlank(lines[i])) rows.push(cells(lines[i++]));
      out.push("<table><thead><tr>" + head.map((c) => `<th>${mdInline(c)}</th>`).join("") + "</tr></thead><tbody>" +
        rows.map((r) => "<tr>" + r.map((c) => `<td>${mdInline(c)}</td>`).join("") + "</tr>").join("") + "</tbody></table>");
      continue;
    }
    if (listRe.test(line)) {
      const items = [];
      while (i < lines.length) {
        const lm = lines[i].match(listRe);
        if (lm) { items.push({ indent: lm[1].replace(/\t/g, "    ").length, ordered: /\d/.test(lm[2]), text: lm[3] }); i++; continue; }
        if (!isBlank(lines[i]) && /^\s{2,}\S/.test(lines[i]) && items.length) { items[items.length - 1].text += " " + lines[i].trim(); i++; continue; }
        if (isBlank(lines[i]) && i + 1 < lines.length && listRe.test(lines[i + 1])) { i++; continue; }
        break;
      }
      out.push(renderList(items));
      continue;
    }
    // paragraph
    const buf = [];
    while (i < lines.length && !isBlank(lines[i]) && !/^\s{0,3}(#{1,4}\s|>|`{3,}|~{3,})/.test(lines[i]) && !listRe.test(lines[i]) && !/^\s{0,3}([-*_])(\s*\1){2,}\s*$/.test(lines[i])) {
      buf.push(lines[i++]);
    }
    if (!buf.length) { buf.push(lines[i++]); }
    out.push(`<p>${buf.map((l, k) => mdInline(l.replace(/\s+$/, "")) + (/ {2,}$/.test(buf[k]) && k < buf.length - 1 ? "<br>" : "")).join(" ")}</p>`);
  }
  return out.join("\n");
}

function renderList(items) {
  let html = ""; const stack = [];
  const open = (it) => { const tag = it.ordered ? "ol" : "ul"; stack.push({ tag, indent: it.indent }); html += `<${tag}>`; };
  for (const it of items) {
    while (stack.length && it.indent < stack[stack.length - 1].indent) { html += `</li></${stack.pop().tag}>`; }
    if (!stack.length) open(it);
    else if (it.indent > stack[stack.length - 1].indent) open(it);
    else html += "</li>";
    let text = it.text, box = "";
    const tm = text.match(/^\[( |x|X)\]\s+(.*)$/);
    if (tm) { box = tm[1] === " " ? "☐ " : "☑ "; text = tm[2]; }
    html += `<li>${box}${mdInline(text)}`;
  }
  while (stack.length) html += `</li></${stack.pop().tag}>`;
  return html;
}

/* ---------- state ---------- */
const S = {
  cfg: { profiles: [], selected: "" },
  gameFolder: "",
  status: {},     // id -> {s:'checking'|'online'|'offline', ms}
  notes: {},      // id -> Notes
  version: "",
  busy: false,
};

const current = () => S.cfg.profiles.find((p) => p.id === S.cfg.selected);

async function save() {
  const st = await api("/api/save", S.cfg);
  S.cfg = st.config; if (!S.cfg.profiles) S.cfg.profiles = []; S.gameFolder = st.gameFolder;
}

/* ---------- rendering ---------- */
function renderRail() {
  const nav = $("#servers");
  nav.innerHTML = S.cfg.profiles.map((p) => {
    const st = (S.status[p.id] || { s: "checking" }).s;
    return `<button class="server" data-id="${esc(p.id)}" data-school="${esc(p.school)}" aria-current="${p.id === S.cfg.selected}">
      <span class="server-name">${esc(p.name)}</span>
      <span class="dot" data-s="${st}" title="${st === "online" ? "Online" : st === "offline" ? "Offline" : "Checking"}"></span>
      <span class="server-addr">${esc(p.address)}:${p.port}</span>
    </button>`;
  }).join("");
}

function renderStage() {
  const stage = $("#stage");
  const p = current();
  if (!p) {
    stage.innerHTML = `<div class="firstrun"><h1>Add your first server</h1><p>A server needs a name, an address and a port. The patch notes address is optional.</p><div><button class="btn btn-solid" id="firstAdd" data-school="storm">Add a server</button></div></div>`;
    $("#firstAdd").onclick = () => editProfile(null);
    return;
  }
  const st = S.status[p.id] || { s: "checking" };
  const stText = st.s === "online" ? `Online${st.ms ? ", " + st.ms + " ms" : ""}` : st.s === "offline" ? "Can't reach this server" : "Checking…";
  const folder = gameFolderOf(p);
  stage.dataset.school = p.school;
  stage.innerHTML = `
    <header class="head">
      <div class="head-main">
        <h1 class="title" title="${esc(p.name)}">${esc(p.name)}</h1>
        <div class="sub"><span>${esc(p.address)}:${p.port}</span><span class="status"><span class="dot" data-s="${st.s}"></span>${stText}</span></div>
      </div>
      <div class="head-actions">
        ${p.bugUrl ? `<button class="link" id="bugBtn">Report a bug</button>` : ""}
        <button class="link" id="shareBtn">Share</button>
        <button class="link" id="editBtn">Edit</button>
      </div>
    </header>
    <section class="pagewrap">
      <div class="page-meta"><span class="where" id="notesWhere"></span><button class="link" id="refreshNotes">Refresh</button></div>
      <div class="page" id="page"></div>
    </section>
    <footer class="bar">
      <div class="bar-info">
        <div class="row ${folder ? "" : "warn"}" id="folderRow">${folder ? "Game: " + esc(folder) : "Wizard101 wasn't found."} <button class="link" id="folderBtn">${folder ? "Change" : "Choose its folder"}</button></div>
        <div class="row" id="patchRow">${p.patchUrl ? (p.autoPatch ? "Game files update when you press Play." : "This server can send game files.") : ""}</div>
      </div>
      ${p.patchUrl ? `<button class="btn btn-ghost" id="patchBtn">Check for patches</button>` : ""}
      <button class="btn btn-solid play" id="playBtn" ${folder ? "" : "disabled"}>Play</button>
    </footer>`;
  $("#editBtn").onclick = () => editProfile(p);
  $("#shareBtn").onclick = () => shareProfile(p);
  if (p.bugUrl) $("#bugBtn").onclick = () => api("/api/open", { url: p.bugUrl }).catch((er) => toast(er.message));
  $("#refreshNotes").onclick = () => loadNotes(p, true);
  $("#folderBtn").onclick = () => openSettings();
  $("#playBtn").onclick = () => play(p);
  if (p.patchUrl) $("#patchBtn").onclick = () => patchFlow(p, false);
  renderNotes(p);
}

function gameFolderOf(p) {
  // The server tells us the folder it will use for the selected profile.
  return S.gameFolder;
}

function renderNotes(p) {
  const page = $("#page"); if (!page) return;
  const n = S.notes[p.id];
  const where = $("#notesWhere");
  if (!p.notesUrl) {
    where.textContent = "";
    $("#refreshNotes").hidden = true;
    page.innerHTML = `<div class="empty"><h3>No patch notes yet</h3><p>Add the address of a text or Markdown file in this server's settings and its notes will show here.</p></div>`;
    return;
  }
  $("#refreshNotes").hidden = false;
  if (!n) { where.textContent = "Loading patch notes…"; page.innerHTML = ""; return; }
  let host = ""; try { host = new URL(p.notesUrl).host; } catch {}
  if (n.markdown) {
    where.textContent = `Patch notes from ${host}, ${n.fromCache ? "last saved " : "refreshed "}${ago(n.fetchedAt)}`;
    page.innerHTML = (n.fromCache ? `<p class="notice">Showing the last saved notes. ${esc(n.error || "")}</p>` : "") + `<div class="md">${mdRender(n.markdown)}</div>`;
  } else {
    where.textContent = `Patch notes from ${host}`;
    page.innerHTML = `<div class="empty"><h3>The patch notes didn't load</h3><p>${esc(n.error || "Try again in a moment.")}</p></div>`;
  }
}

async function loadNotes(p, manual) {
  if (!p.notesUrl) { renderNotes(p); return; }
  if (manual) { delete S.notes[p.id]; renderNotes(p); }
  try {
    S.notes[p.id] = await api("/api/notes?id=" + encodeURIComponent(p.id));
  } catch (e) { S.notes[p.id] = { error: e.message }; }
  if (current() && current().id === p.id) renderNotes(p);
}

async function pingAll() {
  await Promise.all(S.cfg.profiles.map(async (p) => {
    try {
      const r = await api("/api/ping?id=" + encodeURIComponent(p.id));
      S.status[p.id] = { s: r.online ? "online" : "offline", ms: r.ms };
    } catch { S.status[p.id] = { s: "offline" }; }
  }));
  renderRail();
  const p = current(); if (p) refreshHeadStatus(p);
}

function refreshHeadStatus(p) {
  const el = document.querySelector(".status"); if (!el) return;
  const st = S.status[p.id] || { s: "checking" };
  el.innerHTML = `<span class="dot" data-s="${st.s}"></span>` + (st.s === "online" ? `Online${st.ms ? ", " + st.ms + " ms" : ""}` : st.s === "offline" ? "Can't reach this server" : "Checking…");
}

async function select(id) {
  if (S.cfg.selected === id) return;
  S.cfg.selected = id;
  await save().catch(() => {});
  renderRail(); renderStage();
  const p = current(); if (p) loadNotes(p, false);
}

/* ---------- modals ---------- */
function openModal(html, onMount) {
  const root = $("#modalRoot");
  root.innerHTML = `<div class="scrim" id="scrim"><div class="modal" role="dialog" aria-modal="true">${html}</div></div>`;
  const close = () => { root.innerHTML = ""; document.removeEventListener("keydown", onKey); };
  const onKey = (e) => { if (e.key === "Escape" && !$("#scrim").dataset.locked) close(); };
  document.addEventListener("keydown", onKey);
  $("#scrim").addEventListener("mousedown", (e) => { if (e.target.id === "scrim" && !$("#scrim").dataset.locked) close(); });
  const m = root.querySelector(".modal"); const cur = current(); if (m && cur) m.dataset.school = cur.school;
  const first = root.querySelector("input,button,textarea"); if (first) first.focus();
  onMount && onMount(close);
  return close;
}

function editProfile(p, preset) {
  const isNew = !p;
  const d = p || preset || { name: "", address: "", port: 12000, notesUrl: "", patchUrl: "", bugUrl: "", autoPatch: false, school: "storm" };
  const html = `
    <h2>${isNew ? "Add a server" : "Edit server"}</h2>
    <p class="lead">${isNew ? "Where you play, and where its news lives." : "Changes apply the next time you select it."}</p>
    <p class="err" id="formErr" hidden></p>
    <div class="field"><label for="fName">Server name</label><input type="text" id="fName" value="${esc(d.name)}" maxlength="60" autocomplete="off"></div>
    <div class="row2">
      <div class="field"><label for="fAddr">Address</label><input type="text" id="fAddr" value="${esc(d.address)}" placeholder="play.example.com" autocomplete="off" spellcheck="false"></div>
      <div class="field"><label for="fPort">Port</label><input type="number" id="fPort" value="${d.port}" min="1" max="65535"></div>
    </div>
    <div class="field"><span class="lab">School colour</span>
      <div class="schools" role="radiogroup" aria-label="School colour">${SCHOOLS.map((s) => `<label class="school" data-school="${s}"><input type="radio" name="school" value="${s}" ${d.school === s ? "checked" : ""}><span>${s}</span></label>`).join("")}</div>
    </div>
    <div class="field"><label for="fNotes">Patch notes file</label><input type="text" id="fNotes" value="${esc(d.notesUrl)}" placeholder="https://example.com/patchnotes.md" autocomplete="off" spellcheck="false">
      <span class="hint">The address of a Markdown or plain text file. It's fetched each time you pick this server.</span></div>
    <div class="field"><label for="fPatch">Patch files folder <span style="font-weight:400;color:var(--muted)">(optional)</span></label><input type="text" id="fPatch" value="${esc(d.patchUrl)}" placeholder="https://example.com/patch/" autocomplete="off" spellcheck="false">
      <span class="hint">A web folder with a manifest.json that lists game files for this server.</span></div>
    <div class="field"><label for="fBug">Report a bug address <span style="font-weight:400;color:var(--muted)">(optional)</span></label><input type="text" id="fBug" value="${esc(d.bugUrl || "")}" placeholder="https://github.com/you/your-repo/issues/new" autocomplete="off" spellcheck="false">
      <span class="hint">Adds a "Report a bug" button for this server. It opens this page in your browser, and it goes along when you share the server.</span></div>
    <label class="check" id="autoWrap"><input type="checkbox" id="fAuto" ${d.autoPatch ? "checked" : ""}><span>Update game files from this server when I press Play</span></label>
    <div class="modal-foot">
      ${isNew ? "" : `<button class="link danger" id="delBtn">Delete this server</button>`}
      <span class="grow"></span>
      <button class="btn btn-ghost" id="cancelBtn">Cancel</button>
      <button class="btn btn-solid" id="saveBtn">${isNew ? "Add server" : "Save changes"}</button>
    </div>`;
  openModal(html, (close) => {
    const root = $("#modalRoot");
    const tint = () => { const s = (root.querySelector("input[name=school]:checked") || {}).value || "storm"; $(".modal").dataset.school = s; };
    root.querySelectorAll("input[name=school]").forEach((r) => r.addEventListener("change", tint)); tint();
    const syncAuto = () => { const has = $("#fPatch").value.trim() !== ""; $("#fAuto").disabled = !has; $("#autoWrap").setAttribute("aria-disabled", String(!has)); };
    $("#fPatch").addEventListener("input", syncAuto); syncAuto();
    $("#cancelBtn").onclick = close;
    if (!isNew) $("#delBtn").onclick = async () => {
      if (!confirm(`Delete "${d.name}" from your list?`)) return;
      S.cfg.profiles = S.cfg.profiles.filter((x) => x.id !== p.id);
      if (S.cfg.selected === p.id) S.cfg.selected = S.cfg.profiles[0] ? S.cfg.profiles[0].id : "";
      try { await save(); } catch (e) { toast(e.message); }
      close(); afterListChange();
    };
    $("#saveBtn").onclick = async () => {
      const prof = {
        id: p ? p.id : "", name: $("#fName").value, address: $("#fAddr").value, port: parseInt($("#fPort").value, 10) || 0,
        notesUrl: $("#fNotes").value, patchUrl: $("#fPatch").value, bugUrl: $("#fBug").value, autoPatch: $("#fAuto").checked && $("#fPatch").value.trim() !== "",
        school: (root.querySelector("input[name=school]:checked") || {}).value || "storm", gameFolder: p ? p.gameFolder : "",
      };
      const before = JSON.stringify(S.cfg);
      if (p) S.cfg.profiles = S.cfg.profiles.map((x) => (x.id === p.id ? prof : x));
      else { S.cfg.profiles.push(prof); }
      try {
        await save();
        if (!p) { S.cfg.selected = S.cfg.profiles[S.cfg.profiles.length - 1].id; await save(); }
        close(); delete S.notes[(current() || {}).id]; afterListChange(true);
      } catch (e) {
        S.cfg = JSON.parse(before);
        const el = $("#formErr"); el.textContent = e.message; el.hidden = false;
      }
    };
  });
}

function afterListChange(reloadNotes) {
  renderRail(); renderStage();
  const p = current();
  if (p) { loadNotes(p, false); }
  pingAll();
}

async function shareProfile(p) {
  try {
    const r = await api("/api/share?id=" + encodeURIComponent(p.id));
    openModal(`
      <h2>Share ${esc(p.name)}</h2>
      <p class="lead">Send this text to a friend. In their launcher they choose "Paste a shared server".</p>
      <div class="field"><textarea id="shareText" readonly spellcheck="false">${esc(r.code)}</textarea></div>
      <div class="modal-foot"><span class="grow"></span><button class="btn btn-ghost" id="closeBtn">Close</button><button class="btn btn-solid" id="copyBtn">Copy</button></div>`, (close) => {
      $("#closeBtn").onclick = close;
      $("#copyBtn").onclick = async () => {
        const ta = $("#shareText"); ta.select();
        try { await navigator.clipboard.writeText(ta.value); } catch { document.execCommand("copy"); }
        toast("Copied. Paste it into a message."); close();
      };
    });
  } catch (e) { toast(e.message); }
}

function pasteShared() {
  openModal(`
    <h2>Paste a shared server</h2>
    <p class="lead">Paste the text a friend sent you. It starts with w101srv:</p>
    <p class="err" id="formErr" hidden></p>
    <div class="field"><textarea id="pasteText" spellcheck="false" placeholder="w101srv:…"></textarea></div>
    <div class="modal-foot"><span class="grow"></span><button class="btn btn-ghost" id="cancelBtn">Cancel</button><button class="btn btn-solid" id="addBtn">Continue</button></div>`, (close) => {
    $("#cancelBtn").onclick = close;
    $("#addBtn").onclick = async () => {
      try {
        const prof = await api("/api/import", { text: $("#pasteText").value });
        close(); editProfile(null, prof);
      } catch (e) { const el = $("#formErr"); el.textContent = e.message; el.hidden = false; }
    };
  });
}

function openSettings() {
  openModal(`
    <h2>Settings</h2>
    <p class="lead">Applies to every server.</p>
    <p class="err" id="formErr" hidden></p>
    <div class="field"><label for="sFolder">Wizard101 folder</label>
      <div class="folder"><input type="text" id="sFolder" value="${esc(S.cfg.gameFolder || S.gameFolder || "")}" spellcheck="false" autocomplete="off"><button class="btn btn-ghost" id="browseBtn">Browse</button></div>
      <span class="hint" id="folderHint">The folder that contains Bin. <button class="link" id="detectBtn">Find it for me</button></span></div>
    <label class="check"><input type="checkbox" id="sClose" ${S.cfg.closeOnLaunch ? "checked" : ""}><span>Close the launcher after the game starts</span></label>
    <p class="hint">Paste into the game's chat: while the launcher is open, press Ctrl+Shift+V in the game. One line is typed into your open chat box. Several lines are each sent as a message (close the chat first). If you want this, leave the box above unticked.</p>
    <div class="modal-foot"><span class="grow" style="color:var(--muted);font-size:12.5px">Spiral Navigator ${esc(S.version)}</span><button class="btn btn-ghost" id="cancelBtn">Cancel</button><button class="btn btn-solid" id="saveBtn">Save</button></div>`, (close) => {
    const hint = (t, bad) => { $("#folderHint").innerHTML = (bad ? `<span style="color:var(--bad)">${esc(t)}</span>` : esc(t)); };
    $("#cancelBtn").onclick = close;
    $("#browseBtn").onclick = async () => {
      const r = await api("/api/browse", { start: $("#sFolder").value }).catch(() => ({}));
      if (r.folder) $("#sFolder").value = r.folder; else if (r.folder === "") toast("Type the folder path instead, or use Find it for me.");
    };
    $("#detectBtn").onclick = async () => {
      const r = await api("/api/game", { folder: "" }).catch(() => ({}));
      if (r.found) { $("#sFolder").value = r.folder; hint("Found Wizard101."); } else hint("Couldn't find Wizard101. Type the folder path instead.", true);
    };
    $("#saveBtn").onclick = async () => {
      const f = $("#sFolder").value.trim();
      if (f) {
        const r = await api("/api/game", { folder: f }).catch(() => ({}));
        if (!r.found) { const el = $("#formErr"); el.textContent = "That folder doesn't contain Bin\\WizardGraphicalClient.exe."; el.hidden = false; return; }
      }
      S.cfg.gameFolder = f; S.cfg.closeOnLaunch = $("#sClose").checked;
      try { await save(); } catch (e) { toast(e.message); return; }
      close(); renderStage(); const p = current(); if (p) renderNotes(p);
    };
  });
}

/* ---------- patches ---------- */
async function patchFlow(p, thenPlay) {
  let close;
  const body = (inner) => { const m = $(".modal"); if (m) m.innerHTML = inner; };
  close = openModal(`<h2>Patches from ${esc(p.name)}</h2><p class="lead" id="pLead">Checking what's changed…</p>`);
  const scrim = $("#scrim"); scrim.dataset.locked = "1";
  let check;
  try { check = await api("/api/patch/check?id=" + encodeURIComponent(p.id)); }
  catch (e) {
    body(`<h2>Patches from ${esc(p.name)}</h2><p class="err">${esc(e.message)}</p><div class="modal-foot"><span class="grow"></span>${thenPlay ? `<button class="btn btn-ghost" id="playAnyway">Play without patching</button>` : ""}<button class="btn btn-solid" id="okBtn">Close</button></div>`);
    delete scrim.dataset.locked; $("#okBtn").onclick = close; if (thenPlay) $("#playAnyway").onclick = () => { close(); launch(p); };
    return;
  }
  check.files = check.files || [];
  if (!check.files.length) {
    if (thenPlay) { close(); launch(p); return; }
    body(`<h2>You're up to date</h2><p class="lead">All ${check.total} files from ${esc(p.name)} are current${check.version ? " (version " + esc(check.version) + ")" : ""}.</p><div class="modal-foot"><span class="grow"></span><button class="btn btn-solid" id="okBtn">Close</button></div>`);
    delete scrim.dataset.locked; $("#okBtn").onclick = close; return;
  }
  const list = check.files.map((f) => `<li><span>${esc(f.path)}</span><span>${bytes(f.size)}</span></li>`).join("");
  const showList = `<h2>Install ${check.files.length} ${check.files.length === 1 ? "update" : "updates"}?</h2>
    <p class="lead">${esc(p.name)} wants to change these files in your Wizard101 folder (${bytes(check.bytes)} to download). Each file is checked against a checksum before it replaces yours.</p>
    <ul class="filelist">${list}</ul>
    <div class="modal-foot"><span class="grow"></span><button class="btn btn-ghost" id="noBtn">${thenPlay ? "Play without patching" : "Not now"}</button><button class="btn btn-solid" id="yesBtn">Install updates</button></div>`;
  const run = async () => {
    body(`<h2>Installing updates</h2><p class="lead" id="pFile">Starting…</p><div class="progress"><i id="pBar"></i></div><p class="lead" id="pNum"></p>`);
    try { await api("/api/patch/start", { id: p.id }); } catch (e) { body(`<h2>Installing updates</h2><p class="err">${esc(e.message)}</p><div class="modal-foot"><span class="grow"></span><button class="btn btn-solid" id="okBtn">Close</button></div>`); delete scrim.dataset.locked; $("#okBtn").onclick = close; return; }
    for (;;) {
      await new Promise((r) => setTimeout(r, 400));
      const s = await api("/api/patch/status").catch(() => null);
      if (!s) continue;
      const pct = s.total ? Math.min(100, (s.done / s.total) * 100) : 0;
      const bar = $("#pBar"); if (bar) bar.style.width = pct + "%";
      const f = $("#pFile"), n = $("#pNum");
      if (f) f.textContent = s.file ? `File ${s.fileIndex} of ${s.fileCount}: ${s.file}` : "Starting…";
      if (n) n.textContent = `${bytes(s.done)} of ${bytes(s.total)}`;
      if (s.state === "error") {
        body(`<h2>The update stopped</h2><p class="err">${esc(s.error)}</p><div class="modal-foot"><span class="grow"></span>${thenPlay ? `<button class="btn btn-ghost" id="playAnyway">Play anyway</button>` : ""}<button class="btn btn-solid" id="okBtn">Close</button></div>`);
        delete scrim.dataset.locked; $("#okBtn").onclick = close; if (thenPlay) $("#playAnyway").onclick = () => { close(); launch(p); }; return;
      }
      if (s.state === "done") {
        if (thenPlay) { close(); launch(p); return; }
        body(`<h2>Updated</h2><p class="lead">Your game files match ${esc(p.name)}.</p><div class="modal-foot"><span class="grow"></span><button class="btn btn-solid" id="okBtn">Close</button></div>`);
        delete scrim.dataset.locked; $("#okBtn").onclick = close; return;
      }
    }
  };
  body(showList); delete scrim.dataset.locked;
  $("#yesBtn").onclick = run;
  $("#noBtn").onclick = () => { close(); if (thenPlay) launch(p); };
}

/* ---------- playing ---------- */
async function play(p) {
  if (S.busy) return;
  if (p.patchUrl && p.autoPatch) { patchFlow(p, true); return; }
  launch(p);
}

async function launch(p, force) {
  if (S.busy) return;
  S.busy = true;
  const btn = $("#playBtn"); if (btn) { btn.disabled = true; btn.textContent = "Starting…"; }
  try {
    const r = await api("/api/play", { id: p.id, force: !!force });
    toast("Wizard101 is starting.");
    if (r.closeOnLaunch) setTimeout(() => api("/api/quit", {}).catch(() => {}), 1500);
  } catch (e) {
    if (e.data && e.data.offline) {
      openModal(`<h2>Server not reachable</h2><p class="lead">${esc(e.message)}</p><div class="modal-foot"><span class="grow"></span><button class="btn btn-ghost" id="tryBtn">Start the game anyway</button><button class="btn btn-solid" id="okBtn">Close</button></div>`, (close) => {
        $("#okBtn").onclick = close; $("#tryBtn").onclick = () => { close(); launch(p, true); };
      });
    } else toast(e.message);
  } finally {
    S.busy = false;
    const b = $("#playBtn"); if (b) { b.disabled = !S.gameFolder; b.textContent = "Play"; }
  }
}

/* ---------- start ---------- */
document.addEventListener("click", (e) => {
  const a = e.target.closest("a[data-ext]");
  if (a) { e.preventDefault(); api("/api/open", { url: a.getAttribute("href") }).catch((er) => toast(er.message)); return; }
  const s = e.target.closest(".server");
  if (s) select(s.dataset.id);
});
$("#addServer").onclick = () => editProfile(null);
$("#pasteServer").onclick = pasteShared;
$("#openSettings").onclick = openSettings;
document.addEventListener("contextmenu", (e) => { if (!e.target.closest("input,textarea,.page")) e.preventDefault(); });

(async function init() {
  try {
    const st = await api("/api/state");
    S.cfg = st.config; if (!S.cfg.profiles) S.cfg.profiles = []; S.gameFolder = st.gameFolder; S.version = st.version;
  } catch (e) { $("#stage").innerHTML = `<div class="firstrun"><h1>The launcher can't start</h1><p>${esc(e.message)}</p></div>`; return; }
  renderRail(); renderStage();
  const p = current(); if (p) loadNotes(p, false);
  pingAll();
  setInterval(pingAll, 30000);
  setInterval(() => api("/api/beat", {}).catch(() => {}), 5000);
  api("/api/beat", {}).catch(() => {});
})();
