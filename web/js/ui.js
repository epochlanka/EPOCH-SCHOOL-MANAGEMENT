/* Shared UI helpers for Epoch School Connect */
'use strict';

const $ = (s, el = document) => el.querySelector(s);
const $$ = (s, el = document) => [...el.querySelectorAll(s)];
const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));

const S = { me: null, school: {}, children: [], classes: null, routes: null, perms: [], roles: [], classCache: {} };
const can = (p) => S.perms.includes(p);
const roleName = (r) => (S.roles.find((x) => x.id === r) || {}).label || cap(r);

/* ---------- Icons (24px stroke) ---------- */
const ICONS = {
  home: '<path d="M3 10.5 12 3l9 7.5V20a1 1 0 0 1-1 1h-5v-6H9v6H4a1 1 0 0 1-1-1z"/>',
  grid: '<rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/>',
  megaphone: '<path d="M3 11v2a1 1 0 0 0 1 1h2l5 4V6L6 10H4a1 1 0 0 0-1 1z"/><path d="M15.5 8.5a5 5 0 0 1 0 7M18.5 5.5a9 9 0 0 1 0 13"/>',
  calendar: '<rect x="3" y="4.5" width="18" height="16" rx="2"/><path d="M3 9.5h18M8 2.5v4M16 2.5v4"/><path d="m9 15 2 2 4-4"/>',
  users: '<circle cx="9" cy="8" r="3.5"/><path d="M2.5 20a6.5 6.5 0 0 1 13 0"/><circle cx="17" cy="9" r="2.5"/><path d="M16 14.2a5 5 0 0 1 5.5 4.8"/>',
  student: '<path d="m2 9 10-5 10 5-10 5z"/><path d="M6 11v5c0 1.5 3 3 6 3s6-1.5 6-3v-5"/>',
  money: '<rect x="2.5" y="5.5" width="19" height="13" rx="2"/><circle cx="12" cy="12" r="2.8"/><path d="M6 9v.01M18 15v.01"/>',
  book: '<path d="M4 19.5V5a2 2 0 0 1 2-2h14v16H6a2 2 0 0 0-2 2.5z"/><path d="M4 19.5A2 2 0 0 0 6 21h14"/><path d="M9 7h7M9 11h5"/>',
  exam: '<path d="M14 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z"/><path d="M14 3v6h6M8 13h8M8 17h5"/>',
  bus: '<rect x="4" y="3" width="16" height="15" rx="3"/><path d="M4 11h16M8 18v2.5M16 18v2.5"/><circle cx="8" cy="14.5" r="1"/><circle cx="16" cy="14.5" r="1"/>',
  chat: '<path d="M21 12a8 8 0 0 1-11.8 7L3 21l2-5.6A8 8 0 1 1 21 12z"/><path d="M8 11h.01M12 11h.01M16 11h.01"/>',
  chart: '<path d="M3 3v18h18"/><path d="M7 15v3M11 10v8M15 12v6M19 6v12"/>',
  settings: '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z"/>',
  bell: '<path d="M18 8a6 6 0 1 0-12 0c0 7-3 9-3 9h18s-3-2-3-9"/><path d="M13.7 21a2 2 0 0 1-3.4 0"/>',
  sparkle: '<path d="M12 3l1.9 5.1L19 10l-5.1 1.9L12 17l-1.9-5.1L5 10l5.1-1.9z"/><path d="M19 15l.8 2.2L22 18l-2.2.8L19 21l-.8-2.2L16 18l2.2-.8z"/>',
  layers: '<path d="m12 2 10 5-10 5L2 7z"/><path d="m2 17 10 5 10-5M2 12l10 5 10-5"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  logout: '<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4M16 17l5-5-5-5M21 12H9"/>',
  menu: '<path d="M3 6h18M3 12h18M3 18h18"/>',
  send: '<path d="M22 2 11 13M22 2l-7 20-4-9-9-4z"/>',
  trash: '<path d="M3 6h18M8 6V4h8v2M19 6l-1 14H6L5 6"/>',
  edit: '<path d="M12 20h9M16.5 3.5a2.1 2.1 0 1 1 3 3L7 19l-4 1 1-4z"/>',
  check: '<path d="M20 6 9 17l-5-5"/>',
  x: '<path d="M18 6 6 18M6 6l12 12"/>',
  sms: '<rect x="3" y="4" width="18" height="14" rx="3"/><path d="M7 21l3-3"/><path d="M7.5 9.5h9M7.5 13h6"/>',
  whatsapp: '<path d="M3.5 20.5 5 16a8.5 8.5 0 1 1 3.2 3.1z"/><path d="M9 8.5c0 3.5 3 6.5 6.5 6.5l1.2-1.6-2-1-1 .8a5 5 0 0 1-2.4-2.4l.8-1-1-2z"/>',
  phone: '<rect x="6" y="2" width="12" height="20" rx="2.5"/><path d="M11 18h2"/>',
  clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
  alert: '<path d="M12 3 2 20h20z"/><path d="M12 10v4M12 17h.01"/>',
  shield: '<path d="M12 3 4 6v6c0 5 3.5 8 8 9 4.5-1 8-4 8-9V6z"/><path d="m9 12 2 2 4-4"/>',
  user: '<circle cx="12" cy="8" r="4"/><path d="M4 21a8 8 0 0 1 16 0"/>',
  translate: '<path d="M4 5h8M8 3v2M6 5c0 4 3 7 6 8M10 5c0 3-3 7-6 8"/><path d="m13 21 4-9 4 9M14.5 18h5"/>',
  refresh: '<path d="M21 12a9 9 0 1 1-3-6.7L21 8"/><path d="M21 3v5h-5"/>',
  moon: '<path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z"/>',
  diary: '<path d="M5 3h12a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5z"/><path d="M5 3v18M9 8h6M9 12h6"/>',
  trophy: '<path d="M8 21h8M12 17v4M7 4h10v5a5 5 0 0 1-10 0z"/><path d="M17 5h3v2a3 3 0 0 1-3 3M7 5H4v2a3 3 0 0 0 3 3"/>',
};
const icon = (name, cls = '') => `<svg class="${cls}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${ICONS[name] || ''}</svg>`;

const CH = {
  app: { label: 'App', color: 'var(--appc)', icon: 'bell' },
  sms: { label: 'SMS', color: 'var(--sms)', icon: 'sms' },
  whatsapp: { label: 'WhatsApp', color: 'var(--wa)', icon: 'whatsapp' },
};
const chBadge = (c) => `<span class="badge ${c === 'whatsapp' ? 'wa' : c}">${esc(CH[c]?.label || c)}</span>`;

const CAT = {
  attendance: ['calendar', 'tint-green'], fees: ['money', 'tint-red'], homework: ['book', 'tint-orange'],
  exams: ['exam', 'tint-violet'], results: ['trophy', 'tint-violet'], transport: ['bus', 'tint-teal'],
  announcement: ['megaphone', 'tint-blue'], message: ['chat', 'tint-blue'], event: ['megaphone', 'tint-orange'],
  leave: ['calendar', 'tint-orange'], substitution: ['users', 'tint-violet'], staff: ['users', 'tint-teal'], library: ['book', 'tint-teal'],
};
const catIcon = (c) => { const [i, t] = CAT[c] || ['bell', 'tint-blue']; return `<div class="ic ${t}">${icon(i)}</div>`; };

const STATUS_BADGE = {
  present: 'green', absent: 'red', late: 'amber', paid: 'green', unpaid: 'amber', overdue: 'red',
  sent: 'green', delivered: 'green', simulated: 'blue', queued: 'amber', failed: 'red', skipped: '',
  leave: 'blue', pending: 'amber', approved: 'green', rejected: 'red', cancelled: '', assigned: 'green', unfilled: 'red',
  enquiry: 'blue', test_scheduled: 'amber', offered: 'green', enrolled: 'green', withdrawn: '', returned: 'green', active: 'amber',
};
const badge = (s, label) => `<span class="badge ${STATUS_BADGE[s] ?? ''}">${esc(label ?? cap(s))}</span>`;
const cap = (s) => String(s ?? '').replace(/_/g, ' ').replace(/^\w/, (c) => c.toUpperCase());

/* ---------- Formatting ---------- */
const money = (v) => 'Rs. ' + Number(v || 0).toLocaleString('en-LK', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
const num = (v) => Number(v || 0).toLocaleString('en-LK');
const todayStr = () => { const d = new Date(); d.setMinutes(d.getMinutes() - d.getTimezoneOffset()); return d.toISOString().slice(0, 10); };
const addDays = (n) => { const d = new Date(); d.setDate(d.getDate() + n); d.setMinutes(d.getMinutes() - d.getTimezoneOffset()); return d.toISOString().slice(0, 10); };
function fmtDate(s) {
  if (!s) return '';
  const d = new Date(String(s).slice(0, 10) + 'T00:00:00');
  return isNaN(d) ? s : localDate(d);
}
function ago(s) {
  if (!s) return '';
  const d = new Date(String(s).replace(' ', 'T'));
  const m = Math.round((Date.now() - d) / 60000);
  if (isNaN(m)) return s;
  const REL = {
    si: { now: 'දැන්', minute: 'මිනිත්තු {n}කට පෙර', hour: 'පැය {n}කට පෙර', day: 'දින {n}කට පෙර' },
    ta: { now: 'இப்போது', minute: '{n} நிமிடங்களுக்கு முன்', hour: '{n} மணி நேரத்துக்கு முன்', day: '{n} நாட்களுக்கு முன்' },
  };
  const rel = (n, unit) => REL[LANG] ? (n === 0 ? REL[LANG].now : REL[LANG][unit].replace('{n}', n))
    : new Intl.RelativeTimeFormat('en', { numeric: 'auto', style: 'short' }).format(-n, unit);
  if (m < 1) return rel(0, 'minute');
  if (m < 60) return rel(m, 'minute');
  if (m < 1440) return rel(Math.round(m / 60), 'hour');
  if (m < 10080) return rel(Math.round(m / 1440), 'day');
  return fmtDate(s);
}
const initials = (n) => String(n || '?').split(/\s+/).map((p) => p[0]).slice(0, 2).join('').toUpperCase();
function greeting() { const h = new Date().getHours(); return t(h < 12 ? 'Good Morning' : h < 17 ? 'Good Afternoon' : 'Good Evening'); }

/* Tiny, safe Markdown renderer for AI answers (escapes first). */
function md(src) {
  const lines = esc(src).split('\n');
  let html = '', list = null;
  const inline = (t) => t.replace(/\*\*(.+?)\*\*/g, '<b>$1</b>').replace(/(^|[^*])\*(?!\s)(.+?)\*/g, '$1<i>$2</i>').replace(/`([^`]+)`/g, '<code>$1</code>');
  const close = () => { if (list) { html += `</${list}>`; list = null; } };
  for (const raw of lines) {
    const l = raw.trimEnd();
    let m;
    if ((m = l.match(/^\s*[-*•]\s+(.*)/))) { if (list !== 'ul') { close(); html += '<ul>'; list = 'ul'; } html += `<li>${inline(m[1])}</li>`; }
    else if ((m = l.match(/^\s*\d+[.)]\s+(.*)/))) { if (list !== 'ol') { close(); html += '<ol>'; list = 'ol'; } html += `<li>${inline(m[1])}</li>`; }
    else if ((m = l.match(/^#{1,4}\s+(.*)/))) { close(); html += `<h4>${inline(m[1])}</h4>`; }
    else if (l.trim() === '') { close(); }
    else if (/^\|?\s*-{3,}/.test(l)) { /* table separator */ }
    else { close(); html += `<p>${inline(l)}</p>`; }
  }
  close();
  return html;
}

/* ---------- API ---------- */
async function api(method, path, body) {
  const opts = { method, headers: { 'Content-Type': 'application/json' }, credentials: 'same-origin' };
  if (body !== undefined) opts.body = JSON.stringify(body);
  let res;
  try { res = await fetch(path, opts); } catch { throw new Error('Cannot reach the server. Check your connection.'); }
  const data = await res.json().catch(() => ({}));
  if (res.status === 401 && path !== '/api/login') { S.me = null; renderLogin(); throw new Error('Session expired. Please sign in again.'); }
  if (!res.ok) throw new Error(data.error || `Request failed (${res.status})`);
  return data;
}
const GET = (p) => api('GET', p);
const POST = (p, b = {}) => api('POST', p, b);
const PUT = (p, b = {}) => api('PUT', p, b);
const DEL = (p) => api('DELETE', p);

/* ---------- Toast & modal ---------- */
function toast(msg, type = '') {
  const el = document.createElement('div');
  el.className = 'toast ' + type;
  el.textContent = msg;
  $('#toasts').append(el);
  setTimeout(() => el.remove(), type === 'error' ? 6000 : 3500);
}

function modal({ title, body, submit = 'Save', wide = false, onSubmit, onOpen, cancel = 'Cancel', footer = true }) {
  const bg = document.createElement('div');
  bg.className = 'modal-bg';
  bg.innerHTML = `<form class="modal ${wide ? 'wide' : ''}" novalidate>
    <div class="modal-h"><h3>${esc(title)}</h3><button type="button" class="icon-btn right" data-close aria-label="Close">${icon('x')}</button></div>
    <div class="modal-b">${body}</div>
    ${footer ? `<div class="modal-f"><button type="button" class="btn" data-close>${esc(cancel)}</button>${onSubmit ? `<button class="btn primary" type="submit">${esc(submit)}</button>` : ''}</div>` : ''}
  </form>`;
  const close = () => bg.remove();
  bg.addEventListener('click', (e) => { if (e.target === bg || e.target.closest('[data-close]')) close(); });
  const form = $('form', bg);
  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    if (!onSubmit) return;
    const btn = $('button[type=submit]', form);
    btn.disabled = true;
    try { if ((await onSubmit(readForm(form), form)) !== false) close(); }
    catch (err) { toast(err.message, 'error'); }
    finally { btn.disabled = false; }
  });
  document.body.append(bg);
  onOpen && onOpen(form, close);
  const first = $('input:not([type=hidden]):not([type=checkbox]),select,textarea', form);
  first && first.focus();
  return { el: bg, form, close };
}

function confirmBox(text, okLabel = 'Confirm') {
  return new Promise((resolve) => {
    modal({ title: 'Please confirm', body: `<p>${esc(text)}</p>`, submit: okLabel, onSubmit: () => resolve(true), onOpen: (f) => $$('[data-close]', f.parentElement).forEach((b) => b.addEventListener('click', () => resolve(false))) });
  });
}

/* ---------- Forms ---------- */
function field(f, v) {
  const val = v ?? f.value ?? '';
  const req = f.required ? 'required' : '';
  const cls = f.full ? 'field full' : 'field';
  let input;
  if (f.type === 'select') {
    input = `<select class="input" name="${f.name}" ${req}>${(f.options || []).map((o) => {
      const [ov, ol] = Array.isArray(o) ? o : [o, o];
      return `<option value="${esc(ov)}" ${String(ov) === String(val) ? 'selected' : ''}>${esc(ol)}</option>`;
    }).join('')}</select>`;
  } else if (f.type === 'textarea') {
    input = `<textarea class="input" name="${f.name}" ${req} placeholder="${esc(f.placeholder || '')}" rows="${f.rows || 4}">${esc(val)}</textarea>`;
  } else if (f.type === 'checkbox') {
    return `<div class="${cls}"><label class="check"><input type="checkbox" name="${f.name}" ${val ? 'checked' : ''}> ${esc(f.label)}</label>${f.hint ? `<span class="hint">${esc(f.hint)}</span>` : ''}</div>`;
  } else {
    input = `<input class="input" type="${f.type || 'text'}" name="${f.name}" value="${esc(val)}" ${req} placeholder="${esc(f.placeholder || '')}" ${f.step ? `step="${f.step}"` : ''} ${f.min !== undefined ? `min="${f.min}"` : ''} autocomplete="${f.autocomplete || 'off'}">`;
  }
  return `<div class="${cls}"><label>${esc(f.label)}${f.required ? '<span aria-hidden="true"> *</span>' : ''}</label>${input}${f.hint ? `<span class="hint">${esc(f.hint)}</span>` : ''}</div>`;
}
const formFields = (fields, values = {}) => `<div class="form-grid">${fields.map((f) => field(f, values[f.name])).join('')}</div>`;

function readForm(form) {
  const out = {};
  for (const el of form.elements) {
    if (!el.name) continue;
    if (el.type === 'checkbox') {
      if (el.dataset.group) { (out[el.dataset.group] ||= []); if (el.checked) out[el.dataset.group].push(el.value); }
      else out[el.name] = el.checked;
    } else if (el.type === 'number') out[el.name] = el.value === '' ? null : Number(el.value);
    else out[el.name] = el.value.trim();
  }
  return out;
}

function table(cols, rows, empty = 'Nothing here yet.') {
  if (!rows.length) return `<div class="empty">${esc(empty)}</div>`;
  return `<div class="table-wrap"><table class="t"><thead><tr>${cols.map((c) => `<th class="${c.num ? 'num' : ''}">${esc(c.label)}</th>`).join('')}</tr></thead>
  <tbody>${rows.map((r, i) => `<tr>${cols.map((c) => `<td class="${c.num ? 'num' : ''}">${c.render ? c.render(r, i) : esc(r[c.key])}</td>`).join('')}</tr>`).join('')}</tbody></table></div>`;
}

const loading = () => '<div class="empty"><div class="spinner" style="margin:auto"></div></div>';

// loadClasses loads classes, optionally limited to the user's own (scope = 'attendance' | 'academic').
async function loadClasses(force, scope = '') {
  if (!S.classCache[scope] || force) S.classCache[scope] = await GET('/api/classes' + (scope ? '?scope=' + scope : ''));
  S.classes = S.classCache[scope];
  return S.classes;
}
async function loadRoutes(force) { if (!S.routes || force) S.routes = await GET('/api/routes'); return S.routes; }
const classOptions = (all) => [...(all ? [[0, all]] : []), ...(S.classes || []).map((c) => [c.id, c.label])];

function setTheme(t) {
  if (t) document.documentElement.dataset.theme = t; else delete document.documentElement.dataset.theme;
  try { t ? localStorage.setItem('theme', t) : localStorage.removeItem('theme'); } catch {}
}
function toggleTheme() {
  const dark = document.documentElement.dataset.theme === 'dark' || (!document.documentElement.dataset.theme && matchMedia('(prefers-color-scheme: dark)').matches);
  setTheme(dark ? 'light' : 'dark');
}
try { const t = localStorage.getItem('theme'); if (t) document.documentElement.dataset.theme = t; } catch {}

/* ---------- Shared AI chat widget (staff & portal) ---------- */
function aiChat(container, suggestions) {
  let convId = '';
  container.innerHTML = `<div class="ai-wrap card">
    <div class="ai-msgs" id="aiMsgs">
      <div class="ai-msg"><div class="ai-bot">${icon('sparkle')}</div><div class="body">
        <p><b>Hi ${esc(S.me.name.split(' ')[0])}, I'm Epoch AI.</b> I can look up live school data and help you understand it.</p>
        <div class="suggests mt">${suggestions.map((s) => `<button type="button">${esc(s)}</button>`).join('')}</div>
      </div></div>
    </div>
    <form class="chat-input" id="aiForm">
      <textarea class="input" name="q" rows="1" placeholder="Ask anything… (English, සිංහල, தமிழ்)" required></textarea>
      <button class="btn ai" type="submit">${icon('send')} Ask</button>
    </form>
  </div>`;
  const msgs = $('#aiMsgs', container), form = $('#aiForm', container), ta = $('textarea', form);
  const add = (role, html) => { const d = document.createElement('div'); d.className = 'ai-msg ' + role; d.innerHTML = (role === 'user' ? `<div class="avatar">${esc(initials(S.me.name))}</div>` : `<div class="ai-bot">${icon('sparkle')}</div>`) + `<div class="body" data-noi18n>${html}</div>`; msgs.append(d); msgs.scrollTop = msgs.scrollHeight; return d; };
  async function ask(q) {
    add('user', `<p>${esc(q)}</p>`);
    const wait = add('bot', '<div class="typing"><span></span><span></span><span></span></div>');
    $('button', form).disabled = true;
    try {
      const r = await POST('/api/ai/chat', { conversation_id: convId, message: q });
      convId = r.conversation_id;
      $('.body', wait).innerHTML = md(r.answer);
    } catch (e) { $('.body', wait).innerHTML = `<p style="color:var(--danger)">${esc(e.message)}</p>`; }
    finally { $('button', form).disabled = false; msgs.scrollTop = msgs.scrollHeight; }
  }
  $$('.suggests button', container).forEach((b) => b.addEventListener('click', () => ask(b.textContent)));
  ta.addEventListener('keydown', (e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); form.requestSubmit(); } });
  form.addEventListener('submit', (e) => { e.preventDefault(); const q = ta.value.trim(); if (!q) return; ta.value = ''; ask(q); });
}

/* ---------- Shared messaging widget ---------- */
async function messenger(container, openId) {
  container.innerHTML = `<div class="card chat"><div class="chat-list" id="convList">${loading()}</div><div class="chat-pane" id="chatPane"><div class="empty" style="margin:auto">Select a conversation or start a new one.</div></div></div>`;
  const list = $('#convList', container), pane = $('#chatPane', container);
  let active = openId || null;
  async function refreshList() {
    const convs = await GET('/api/messages');
    list.innerHTML = `<div style="padding:12px"><button class="btn primary block" id="newChat">${icon('plus')} New message</button></div>` +
      (convs.length ? convs.map((c) => `<div class="c ${c.other_id == active ? 'active' : ''}" data-id="${c.other_id}">
        <div class="avatar">${esc(initials(c.name))}</div>
        <div class="grow"><div class="row"><b>${esc(c.name)}</b><span class="muted small right">${ago(c.created_at)}</span></div>
        <div class="row"><span class="p">${c.sender_id == S.me.id ? 'You: ' : ''}${esc(c.body)}</span>${c.unread ? `<span class="badge red right">${c.unread}</span>` : ''}</div></div></div>`).join('')
        : '<div class="empty">No conversations yet.</div>');
    $$('.c', list).forEach((el) => el.addEventListener('click', () => openThread(el.dataset.id)));
    $('#newChat', list).addEventListener('click', pickContact);
  }
  async function pickContact() {
    const contacts = await GET('/api/messages/contacts');
    modal({
      title: 'New message', submit: 'Open chat',
      body: field({ name: 'to', label: 'To', type: 'select', required: true, options: contacts.map((c) => [c.id, `${c.name} (${cap(c.role)})${c.detail ? ' – ' + c.detail : ''}`]) }),
      onSubmit: (v) => openThread(v.to),
    });
  }
  async function openThread(id) {
    active = id;
    $$('.c', list).forEach((el) => el.classList.toggle('active', el.dataset.id == id));
    pane.innerHTML = loading();
    const t = await GET('/api/messages/thread/' + id);
    pane.innerHTML = `<div class="card-h"><div class="avatar">${esc(initials(t.contact.name))}</div><div><b>${esc(t.contact.name)}</b><div class="muted small">${esc(cap(t.contact.role))}${t.contact.phone ? ' · ' + esc(t.contact.phone) : ''}</div></div></div>
      <div class="chat-msgs" id="msgs">${t.messages.map((m) => `<div class="bubble ${m.sender_id == S.me.id ? 'me' : ''}"><span data-noi18n>${esc(m.body)}</span><small>${ago(m.created_at)}</small></div>`).join('') || '<div class="empty">Say hello 👋</div>'}</div>
      <form class="chat-input" id="sendForm">
        <textarea class="input" name="body" rows="1" placeholder="Type a message…" required></textarea>
        ${S.aiEnabled ? `<button type="button" class="btn ai" id="aiReply" title="Suggest a reply with AI">${icon('sparkle')}</button>` : ''}
        <button class="btn primary" type="submit">${icon('send')}</button>
      </form>`;
    const box = $('#msgs', pane); box.scrollTop = box.scrollHeight;
    const form = $('#sendForm', pane), ta = $('textarea', form);
    ta.addEventListener('keydown', (e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); form.requestSubmit(); } });
    form.addEventListener('submit', async (e) => {
      e.preventDefault();
      const body = ta.value.trim(); if (!body) return;
      try { await POST('/api/messages', { to: Number(id), body }); ta.value = ''; await openThread(id); refreshList(); }
      catch (err) { toast(err.message, 'error'); }
    });
    const aiBtn = $('#aiReply', pane);
    aiBtn && aiBtn.addEventListener('click', async () => {
      aiBtn.disabled = true;
      try { const r = await POST('/api/ai/reply', { user_id: Number(id), hint: ta.value }); ta.value = r.text; ta.focus(); }
      catch (err) { toast(err.message, 'error'); }
      finally { aiBtn.disabled = false; }
    });
    refreshList();
  }
  await refreshList();
  if (active) openThread(active);
}
