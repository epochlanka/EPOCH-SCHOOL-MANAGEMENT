/* Staff console (admin & teacher) */
'use strict';

const NAV = [
  { id: 'dashboard', label: 'Dashboard', icon: 'grid' },
  { section: 'Communication' },
  { id: 'compose', label: 'Send Message', icon: 'megaphone' },
  { id: 'messages', label: 'Messages', icon: 'chat', badge: () => S.unreadMessages },
  { id: 'ai', label: 'AI Assistant', icon: 'sparkle' },
  { id: 'reports', label: 'Reports & History', icon: 'chart' },
  { section: 'School' },
  { id: 'attendance', label: 'Attendance', icon: 'calendar' },
  { id: 'students', label: 'Students', icon: 'student' },
  { id: 'fees', label: 'Fees', icon: 'money' },
  { id: 'homework', label: 'Homework', icon: 'book' },
  { id: 'exams', label: 'Exams & Results', icon: 'exam' },
  { id: 'transport', label: 'Transport', icon: 'bus' },
  { section: 'Administration', admin: true },
  { id: 'users', label: 'Users', icon: 'users', admin: true },
  { id: 'classes', label: 'Classes', icon: 'layers', admin: true },
  { id: 'settings', label: 'Settings', icon: 'settings', admin: true },
];
const PAGES = {};
const isAdmin = () => S.me.role === 'admin';

function renderStaff() {
  const page = (location.hash.slice(2) || 'dashboard').split('/')[0];
  const nav = NAV.filter((n) => !n.admin || isAdmin());
  const item = nav.find((n) => n.id === page) || nav[0];
  $('#app').innerHTML = `<div class="shell" id="shell">
    <aside class="sidebar">
      <div class="side-brand"><img src="assets/logo-mark.png" alt=""><div><b>EPOCH</b><small>${esc(S.school.name)}</small></div></div>
      <nav class="nav">${nav.map((n) => n.section ? `<div class="nav-label">${esc(n.section)}</div>` :
        `<a href="#/${n.id}" class="${n.id === item.id ? 'active' : ''}">${icon(n.icon)}<span>${esc(n.label)}</span>${n.badge && n.badge() ? `<span class="pill">${n.badge()}</span>` : ''}</a>`).join('')}</nav>
      <div class="side-foot">Epoch · Smart Solutions. Better Tomorrow.</div>
    </aside>
    <div class="main">
      <header class="topbar">
        <button class="icon-btn menu-toggle" id="menuBtn" aria-label="Menu">${icon('menu')}</button>
        <div><h1>${esc(item.label)}</h1><div class="crumb">${new Date().toLocaleDateString('en-GB', { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' })}</div></div>
        <div class="spacer"></div>
        <button class="icon-btn" id="themeBtn" title="Toggle dark mode" aria-label="Toggle dark mode">${icon('moon')}</button>
        <button class="icon-btn" id="bellBtn" title="Notifications" aria-label="Notifications">${icon('bell')}${S.unread ? `<span class="dot">${S.unread}</span>` : ''}</button>
        <div class="user-chip" id="userChip" title="My profile"><div class="avatar">${esc(initials(S.me.name))}</div><div class="meta"><b>${esc(S.me.name)}</b><br><small>${esc(S.me.role)}</small></div></div>
        <button class="icon-btn" id="logoutBtn" title="Sign out" aria-label="Sign out">${icon('logout')}</button>
      </header>
      <main class="content" id="page">${loading()}</main>
    </div>
  </div>`;
  $('#menuBtn').onclick = () => $('#shell').classList.toggle('nav-open');
  $('.sidebar').addEventListener('click', (e) => { if (e.target.closest('a')) $('#shell').classList.remove('nav-open'); });
  $('#themeBtn').onclick = toggleTheme;
  $('#logoutBtn').onclick = logout;
  $('#bellBtn').onclick = showNotifications;
  $('#userChip').onclick = profileModal;
  PAGES[item.id]($('#page')).catch((e) => { $('#page').innerHTML = `<div class="card empty">${esc(e.message)}</div>`; });
}

async function showNotifications() {
  const list = await GET('/api/notifications');
  modal({
    title: 'Notifications', footer: false,
    body: list.length ? `<div style="margin:-20px">${list.map((n) => `<div class="list-item">${catIcon(n.category)}<div class="grow"><div class="t1">${esc(n.title)} ${n.is_read ? '' : '<span class="badge blue">New</span>'}</div><div class="t2">${esc(n.body)}</div></div><span class="muted small">${ago(n.created_at)}</span></div>`).join('')}</div>` : '<div class="empty">No notifications.</div>',
  });
  if (S.unread) { await POST('/api/notifications/read-all'); S.unread = 0; const d = $('#bellBtn .dot'); d && d.remove(); }
}

function profileModal() {
  modal({
    title: 'My profile', submit: 'Save',
    body: formFields([
      { name: 'phone', label: 'Mobile number', value: S.me.phone },
      { name: 'whatsapp', label: 'WhatsApp number', value: S.me.whatsapp },
      { name: 'language', label: 'Preferred language', type: 'select', options: ['English', 'Sinhala', 'Tamil'], value: S.me.language },
    ]) + `<hr style="border:0;border-top:1px solid var(--border);margin:6px 0 16px"><h4 style="margin-bottom:10px">Change password</h4>` + formFields([
      { name: 'current', label: 'Current password', type: 'password', autocomplete: 'current-password' },
      { name: 'new', label: 'New password (min 8)', type: 'password', autocomplete: 'new-password' },
    ]),
    onSubmit: async (v) => {
      await PUT('/api/me', v);
      Object.assign(S.me, { phone: v.phone, whatsapp: v.whatsapp, language: v.language });
      if (v.new) await POST('/api/me/password', { current: v.current, new: v.new });
      toast('Profile updated', 'success');
    },
  });
}

/* ---------- Dashboard ---------- */
PAGES.dashboard = async (el) => {
  const d = await GET('/api/dashboard');
  const a = d.attendance, marked = a.present + a.absent + a.late;
  const pct = marked ? Math.round(((a.present + a.late) * 100) / marked) : 0;
  const ch = { app: 0, sms: 0, whatsapp: 0 }, failed = { app: 0, sms: 0, whatsapp: 0 };
  d.channels.forEach((c) => { ch[c.channel] = (ch[c.channel] || 0) + c.n; if (c.status === 'failed') failed[c.channel] += c.n; });
  const chMax = Math.max(1, ...Object.values(ch));
  const deg = (v) => (marked ? (v / marked) * 360 : 0);
  el.innerHTML = `<div class="stack">
    <div class="grid g-4">
      ${stat('student', 'tint-blue', num(d.counts.students), 'Students', `${num(d.counts.classes)} classes · ${num(d.counts.teachers)} teachers`)}
      ${stat('calendar', 'tint-green', marked ? pct + '%' : '—', "Today's attendance", `${a.absent} absent · ${a.late} late · ${a.unmarked} unmarked`)}
      ${stat('money', 'tint-red', 'Rs. ' + num(Math.round(d.fees.outstanding)), 'Outstanding fees', `${d.fees.overdue_count} overdue (${money(d.fees.overdue_amount)})`)}
      ${stat('send', 'tint-violet', num(d.sent_today), 'Messages sent today', `${num(d.counts.parents)} parents connected`)}
    </div>
    <div class="card"><div class="card-b row">
      <b>Quick actions</b><span class="spacer"></span>
      <a class="btn" href="#/attendance">${icon('calendar')} Mark attendance</a>
      <a class="btn" href="#/compose">${icon('megaphone')} Send announcement</a>
      ${isAdmin() ? `<button class="btn" id="qaFees">${icon('money')} Send fee reminders</button>` : ''}
      <a class="btn" href="#/transport">${icon('bus')} Bus update</a>
      <a class="btn ai" href="#/ai">${icon('sparkle')} Ask Epoch AI</a>
    </div></div>
    <div class="grid g-main">
      <div class="card"><div class="card-h"><h3>Attendance trend</h3><span class="muted small">last 2 weeks, % present</span></div>
        <div class="card-b"><div class="bars">${d.trend.map((t) => `<div class="b"><em>${t.marked ? Math.round(t.percent) + '%' : '–'}</em><i style="height:${Math.max(3, t.percent * 1.2)}px;${t.marked ? '' : 'opacity:.25'}"></i><span>${esc(t.label)}</span></div>`).join('')}</div></div></div>
      <div class="card"><div class="card-h"><h3>Today</h3><span class="spacer"></span><a class="btn sm" href="#/attendance">Open</a></div>
        <div class="card-b row" style="gap:22px;flex-wrap:nowrap">
          <div class="donut" style="background:conic-gradient(var(--success) 0 ${deg(a.present)}deg, var(--orange) 0 ${deg(a.present + a.late)}deg, var(--danger) 0 ${deg(marked)}deg, var(--surface-2) 0)"><div><div><b>${marked ? pct + '%' : '—'}</b><div class="muted small">present</div></div></div></div>
          <div class="legend"><div><i style="background:var(--success)"></i>Present <b>${a.present}</b></div><div><i style="background:var(--orange)"></i>Late <b>${a.late}</b></div><div><i style="background:var(--danger)"></i>Absent <b>${a.absent}</b></div><div><i style="background:var(--border)"></i>Unmarked <b>${a.unmarked}</b></div></div>
        </div></div>
    </div>
    <div class="grid g-3">
      <div class="card"><div class="card-h"><h3>Channels</h3><span class="muted small">last 30 days</span></div><div class="card-b">
        ${['whatsapp', 'sms', 'app'].map((c) => `<div class="hbar"><span>${chBadge(c)}</span><div class="track"><div class="fill" style="width:${(ch[c] / chMax) * 100}%;background:${CH[c].color}"></div></div><b class="num">${num(ch[c])}</b></div>${failed[c] ? `<div class="small" style="color:var(--danger);margin:-4px 0 6px 120px">${failed[c]} failed</div>` : ''}`).join('')}
        <a class="btn sm mt" href="#/reports">View delivery report</a>
      </div></div>
      <div class="card"><div class="card-h"><h3>Absent & late today</h3></div>
        ${d.absentees.length ? d.absentees.map((s) => `<div class="list-item"><div class="avatar">${esc(initials(s.name))}</div><div class="grow"><div class="t1">${esc(s.name)}</div><div class="t2">${esc(s.class_name)}</div></div>${badge(s.status)}</div>`).join('') : '<div class="empty">No absentees recorded today 🎉</div>'}
      </div>
      <div class="card"><div class="card-h"><h3>Upcoming exams</h3></div>
        ${d.upcoming_exams.length ? d.upcoming_exams.map((e) => `<div class="list-item">${catIcon('exams')}<div class="grow"><div class="t1">${esc(e.subject)} – ${esc(e.title)}</div><div class="t2">${esc(e.class_name)} · ${fmtDate(e.exam_date)}</div></div></div>`).join('') : '<div class="empty">No upcoming exams.</div>'}
      </div>
    </div>
    <div class="card"><div class="card-h"><h3>Recent communication</h3><span class="spacer"></span><a class="btn sm" href="#/reports">All history</a></div>
      ${table([
        { label: 'Channel', render: (r) => chBadge(r.channel) }, { label: 'Recipient', key: 'user_name' },
        { label: 'Message', render: (r) => `<b>${esc(r.title)}</b>` }, { label: 'Category', render: (r) => esc(cap(r.category)) },
        { label: 'Status', render: (r) => badge(r.status) }, { label: 'When', render: (r) => `<span class="muted">${ago(r.created_at)}</span>` },
      ], d.recent, 'No messages sent yet.')}
    </div>
  </div>`;
  const qa = $('#qaFees', el);
  qa && (qa.onclick = sendDueReminders);
};
function stat(ic, tint, v, l, sub) {
  return `<div class="card stat"><div class="ic ${tint}">${icon(ic)}</div><div style="min-width:0"><div class="l">${esc(l)}</div><div class="v">${esc(v)}</div><div class="muted small">${esc(sub || '')}</div></div></div>`;
}
async function sendDueReminders() {
  if (!(await confirmBox('Send reminders to parents for all fees that are overdue or due soon?', 'Send reminders'))) return;
  try { const r = await POST('/api/fees/remind-due'); toast(`${r.reminded} fee reminder(s) sent`, 'success'); }
  catch (e) { toast(e.message, 'error'); }
}

/* ---------- Compose / announcements ---------- */
PAGES.compose = async (el) => {
  await Promise.all([loadClasses(), loadRoutes()]);
  const [users, history] = await Promise.all([GET('/api/users'), GET('/api/announcements')]);
  el.innerHTML = `<div class="grid g-main">
    <div class="stack">
      ${S.aiEnabled ? `<div class="ai-panel"><div class="row" style="margin-bottom:10px"><div class="ai-bot">${icon('sparkle')}</div><div><b>Write with Epoch AI</b><div class="muted small">Describe the message — AI drafts the title, full message and SMS version.</div></div></div>
        <form id="aiForm"><div class="field"><textarea class="input" name="prompt" rows="2" placeholder="e.g. School closed on Friday for the annual sports meet, parents invited at 2pm"></textarea></div>
        <div class="row"><select class="input" name="tone" style="width:auto"><option>warm and professional</option><option>formal</option><option>urgent</option><option>celebratory</option><option>gentle reminder</option></select>
        <select class="input" name="language" style="width:auto"><option>English</option><option>Sinhala</option><option>Tamil</option><option value="English, followed by a Sinhala and a Tamil translation">Trilingual (EN/SI/TA)</option></select>
        <button class="btn ai right" type="submit">${icon('sparkle')} Generate</button></div></form></div>` : `<div class="card card-b small muted">${icon('sparkle', '')} AI writing is off. Add <code>ANTHROPIC_API_KEY</code> to the server's .env to enable it.</div>`}
      <form class="card" id="sendForm"><div class="card-h"><h3>Compose</h3></div><div class="card-b">
        <div class="form-grid">
          ${field({ name: 'audience', label: 'Send to', type: 'select', options: [['parents', 'All parents'], ['all', 'Everyone'], ['teachers', 'All staff'], ['students', 'All students'], ['class', 'A class (parents + students)'], ['class_parents', 'A class (parents only)'], ['route', 'A bus route'], ['user', 'One person']] })}
          <div class="field" id="audWrap" style="visibility:hidden"><label id="audLabel">Select</label><select class="input" name="audience_id"></select></div>
          ${field({ name: 'category', label: 'Category', type: 'select', options: [['announcement', 'Announcement / Circular'], ['event', 'Event'], ['fees', 'Fees'], ['exams', 'Exams'], ['homework', 'Homework'], ['transport', 'Transport'], ['attendance', 'Attendance']] })}
          ${field({ name: 'title', label: 'Title', required: true, placeholder: 'e.g. School Announcement' })}
          ${field({ name: 'body', label: 'Message', type: 'textarea', required: true, full: true, rows: 6 })}
        </div>
        <div class="row" style="margin:-6px 0 14px">${S.aiEnabled ? ['Sinhala', 'Tamil', 'English'].map((l) => `<button type="button" class="btn sm" data-tr="${l}">${icon('translate')} ${l}</button>`).join('') : ''}<span class="muted small right" id="charCount"></span></div>
        <label style="font-weight:600;font-size:13px">Channels</label>
        <div class="channel-pick mt">${['app', 'sms', 'whatsapp'].map((c) => `<label><input type="checkbox" name="ch_${c}" value="${c}" data-group="channels" ${c !== 'sms' ? 'checked' : ''}><span class="ch-ic" style="background:${CH[c].color}">${icon(CH[c].icon)}</span>${CH[c].label}</label>`).join('')}</div>
        <div class="row mt"><button class="btn primary" type="submit">${icon('send')} Send now</button></div>
      </div></form>
    </div>
    <div class="stack">
      <div class="card"><div class="card-h"><h3>Preview</h3></div><div class="card-b stack" style="gap:14px">
        <div><div class="muted small" style="margin-bottom:6px">${chBadge('app')} App notification</div><div class="list-item card" style="border-radius:12px;padding:12px">${catIcon('announcement')}<div class="grow"><div class="t1" id="pvTitle">Title</div><div class="t2" id="pvBody">Your message…</div></div><span class="muted small">now</span></div></div>
        <div><div class="muted small" style="margin-bottom:6px">${chBadge('whatsapp')} WhatsApp</div><div class="preview-phone"><div class="wa-bubble" id="pvWa"></div></div></div>
        <div><div class="muted small" style="margin-bottom:6px">${chBadge('sms')} SMS <span id="smsParts"></span></div><div class="preview-phone"><div class="sms-bubble" id="pvSms"></div></div></div>
      </div></div>
    </div>
  </div>
  <div class="card mt"><div class="card-h"><h3>Sent announcements</h3></div>
    ${table([
      { label: 'Title', render: (r) => `<b>${esc(r.title)}</b><div class="muted small" style="max-width:420px">${esc(r.body.slice(0, 120))}${r.body.length > 120 ? '…' : ''}</div>` },
      { label: 'Audience', render: (r) => esc(cap(r.audience) + (r.audience_label ? ': ' + r.audience_label : '')) },
      { label: 'Channels', render: (r) => r.channels.split(',').map(chBadge).join(' ') },
      { label: 'Recipients', key: 'recipients', num: true }, { label: 'By', key: 'created_by_name' },
      { label: 'Sent', render: (r) => `<span class="muted">${ago(r.created_at)}</span>` },
    ], history, 'No announcements sent yet.')}
  </div>`;

  const form = $('#sendForm', el);
  const aud = form.elements.audience, audSel = form.elements.audience_id;
  const audOpts = {
    class: ['Class', S.classes.map((c) => [c.id, c.label])], class_parents: ['Class', S.classes.map((c) => [c.id, c.label])],
    route: ['Bus route', S.routes.map((r) => [r.id, r.name])], user: ['Person', users.map((u) => [u.id, `${u.name} (${u.role})`])],
  };
  aud.onchange = () => {
    const o = audOpts[aud.value];
    $('#audWrap', el).style.visibility = o ? 'visible' : 'hidden';
    if (o) { $('#audLabel', el).textContent = o[0]; audSel.innerHTML = o[1].map(([v, l]) => `<option value="${v}">${esc(l)}</option>`).join(''); }
  };
  const preview = () => {
    const t = form.elements.title.value || 'Title', b = form.elements.body.value || 'Your message…';
    $('#pvTitle', el).textContent = t; $('#pvBody', el).textContent = b;
    const full = `${t}\n${b}\n- ${S.school.name}`;
    $('#pvWa', el).textContent = full; $('#pvSms', el).textContent = full;
    const len = full.length, unicode = /[^\x00-\x7F]/.test(full), per = unicode ? 70 : 160;
    $('#smsParts', el).textContent = `· ${len} chars · ${Math.ceil(len / per)} SMS part(s)${unicode ? ' (Unicode)' : ''}`;
    $('#charCount', el).textContent = `${form.elements.body.value.length} characters`;
  };
  form.addEventListener('input', preview); preview();

  $$('[data-tr]', form).forEach((b) => b.onclick = async () => {
    const text = form.elements.body.value.trim(); if (!text) return toast('Write a message first', 'error');
    b.disabled = true;
    try {
      const [t, m] = await Promise.all([POST('/api/ai/translate', { text: form.elements.title.value || ' ', language: b.dataset.tr }), POST('/api/ai/translate', { text, language: b.dataset.tr })]);
      form.elements.title.value = t.text; form.elements.body.value = m.text; preview(); toast('Translated to ' + b.dataset.tr, 'success');
    } catch (e) { toast(e.message, 'error'); } finally { b.disabled = false; }
  });

  const aiForm = $('#aiForm', el);
  aiForm && aiForm.addEventListener('submit', async (e) => {
    e.preventDefault();
    const v = readForm(aiForm), btn = $('button[type=submit]', aiForm);
    if (!v.prompt && !form.elements.body.value) return toast('Describe the message you need', 'error');
    btn.disabled = true; btn.innerHTML = `${icon('sparkle')} Writing…`;
    try {
      const r = await POST('/api/ai/compose', { ...v, audience: aud.options[aud.selectedIndex].text, draft: v.prompt ? '' : form.elements.body.value });
      form.elements.title.value = r.title;
      form.elements.body.value = form.elements.ch_sms.checked && !form.elements.ch_whatsapp.checked && !form.elements.ch_app.checked ? r.sms : r.message;
      form.dataset.sms = r.sms; preview(); toast('Draft ready — review and send', 'success');
    } catch (err) { toast(err.message, 'error'); } finally { btn.disabled = false; btn.innerHTML = `${icon('sparkle')} Generate`; }
  });

  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    const v = readForm(form);
    if (!v.title || !v.body) return toast('Title and message are required', 'error');
    if (!v.channels.length) return toast('Choose at least one channel', 'error');
    const btn = $('button[type=submit]', form); btn.disabled = true;
    try {
      const r = await POST('/api/announcements', { title: v.title, body: v.body, audience: v.audience, audience_id: Number(v.audience_id || 0), channels: v.channels, category: v.category });
      toast(`Sent to ${r.recipients} recipient(s)`, 'success');
      PAGES.compose(el);
    } catch (err) { toast(err.message, 'error'); btn.disabled = false; }
  });
};

/* ---------- Messages & AI ---------- */
PAGES.messages = async (el) => { await messenger(el, location.hash.split('/')[2]); };
PAGES.ai = async (el) => {
  if (!S.aiEnabled) { el.innerHTML = `<div class="card empty"><div class="ai-bot" style="margin:0 auto 12px">${icon('sparkle')}</div><b>Epoch AI is not enabled yet</b><p>Add <code>ANTHROPIC_API_KEY=…</code> to the <code>.env</code> file on the server and restart it.</p></div>`; return; }
  aiChat(el, ['Give me a summary of the school today', 'Which students were absent today?', 'List overdue fees and suggest a reminder message', 'How did the last exams go?', 'Which students have attendance below 85%?', 'How many WhatsApp messages did we send this week?']);
};

/* ---------- Reports ---------- */
PAGES.reports = async (el) => {
  el.innerHTML = `<form class="card card-b row" id="rf">
    ${['from', 'to'].map((n) => `<div class="field" style="margin:0"><label>${cap(n)}</label><input class="input" type="date" name="${n}" value="${n === 'from' ? addDays(-30) : todayStr()}"></div>`).join('')}
    <div class="field" style="margin:0"><label>Channel</label><select class="input" name="channel"><option value="">All</option><option value="app">App</option><option value="sms">SMS</option><option value="whatsapp">WhatsApp</option></select></div>
    <div class="field" style="margin:0"><label>Category</label><select class="input" name="category"><option value="">All</option>${Object.keys(CAT).map((c) => `<option value="${c}">${cap(c)}</option>`).join('')}</select></div>
    <div class="field" style="margin:0"><label>Status</label><select class="input" name="status"><option value="">All</option>${['delivered', 'sent', 'simulated', 'queued', 'failed', 'skipped'].map((c) => `<option value="${c}">${cap(c)}</option>`).join('')}</select></div>
    <button class="btn primary" style="align-self:flex-end">Apply</button>
    <button class="btn" type="button" id="csv" style="align-self:flex-end">Export CSV</button>
  </form><div id="rout" class="mt">${loading()}</div>`;
  let last = [];
  const run = async () => {
    const q = new URLSearchParams(readForm($('#rf', el))).toString();
    const d = await GET('/api/reports/communications?' + q);
    last = d.rows;
    const tot = {}; d.by_channel.forEach((r) => { tot[r.channel] ||= { all: 0, ok: 0, failed: 0 }; tot[r.channel].all += r.n; if (['sent', 'delivered', 'simulated'].includes(r.status)) tot[r.channel].ok += r.n; if (r.status === 'failed') tot[r.channel].failed += r.n; });
    const catMax = Math.max(1, ...d.by_category.map((c) => c.n));
    const dayMax = Math.max(1, ...d.by_day.map((c) => c.n));
    $('#rout', el).innerHTML = `<div class="stack">
      <div class="grid g-3">${['whatsapp', 'sms', 'app'].map((c) => { const t = tot[c] || { all: 0, ok: 0, failed: 0 }; return `<div class="card stat"><div class="ch-ic" style="background:${CH[c].color};width:46px;height:46px;border-radius:12px">${icon(CH[c].icon)}</div><div><div class="l">${CH[c].label}</div><div class="v">${num(t.all)}</div><div class="muted small">${num(t.ok)} delivered · <span style="color:${t.failed ? 'var(--danger)' : 'inherit'}">${num(t.failed)} failed</span></div></div></div>`; }).join('')}</div>
      <div class="grid g-2">
        <div class="card"><div class="card-h"><h3>By category</h3></div><div class="card-b">${d.by_category.map((c) => `<div class="hbar"><span>${esc(cap(c.category))}</span><div class="track"><div class="fill" style="width:${(c.n / catMax) * 100}%;background:var(--brand)"></div></div><b class="num">${num(c.n)}</b></div>`).join('') || '<div class="empty">No data</div>'}</div></div>
        <div class="card"><div class="card-h"><h3>Daily volume</h3></div><div class="card-b"><div class="bars">${d.by_day.slice(-14).map((x) => `<div class="b"><em>${x.n}</em><i style="height:${(x.n / dayMax) * 120 + 3}px"></i><span>${esc(x.day.slice(5))}</span></div>`).join('') || '<div class="empty">No data</div>'}</div></div></div>
      </div>
      <div class="card"><div class="card-h"><h3>Delivery log</h3><span class="muted small">${d.rows.length} most recent</span></div>
      ${table([
        { label: 'When', render: (r) => `<span class="muted small">${esc(r.created_at)}</span>` }, { label: 'Channel', render: (r) => chBadge(r.channel) },
        { label: 'Recipient', render: (r) => `${esc(r.user_name || '—')}<div class="muted small">${esc(r.recipient)}</div>` },
        { label: 'Message', render: (r) => `<b>${esc(r.title)}</b>` }, { label: 'Category', render: (r) => esc(cap(r.category)) },
        { label: 'Status', render: (r) => badge(r.status) + (r.error ? `<div class="small" style="color:var(--danger);max-width:240px">${esc(r.error)}</div>` : '') },
      ], d.rows, 'No messages in this period.')}</div></div>`;
  };
  $('#rf', el).addEventListener('submit', (e) => { e.preventDefault(); run().catch((er) => toast(er.message, 'error')); });
  $('#csv', el).onclick = () => {
    const cols = ['created_at', 'channel', 'user_name', 'recipient', 'category', 'title', 'status', 'error'];
    const csv = [cols.join(','), ...last.map((r) => cols.map((c) => `"${String(r[c] ?? '').replace(/"/g, '""')}"`).join(','))].join('\n');
    const a = document.createElement('a'); a.href = URL.createObjectURL(new Blob([csv], { type: 'text/csv' })); a.download = 'communication-report.csv'; a.click();
  };
  await run();
};

/* ---------- Attendance ---------- */
PAGES.attendance = async (el) => {
  await loadClasses();
  if (!S.classes.length) { el.innerHTML = '<div class="card empty">Create a class first.</div>'; return; }
  el.innerHTML = `<div class="row" style="margin-bottom:16px"><div class="tabs"><button class="active" data-t="mark">Mark attendance</button><button data-t="report">Attendance report</button></div></div><div id="att"></div>`;
  const tabs = $$('.tabs button', el);
  tabs.forEach((b) => b.onclick = () => { tabs.forEach((x) => x.classList.toggle('active', x === b)); (b.dataset.t === 'mark' ? markView : reportView)(); });
  const box = $('#att', el);

  async function markView() {
    box.innerHTML = `<div class="card"><div class="card-h">
      <select class="input" id="cls" style="width:auto">${classOptions().map(([v, l]) => `<option value="${v}">${esc(l)}</option>`).join('')}</select>
      <input class="input" type="date" id="dt" value="${todayStr()}" max="${todayStr()}" style="width:auto">
      <span class="spacer"></span><button class="btn sm" id="allP">${icon('check')} Mark all present</button>
    </div><div id="list">${loading()}</div>
    <div class="card-b row" style="border-top:1px solid var(--border)"><span class="muted small" id="summary"></span><span class="spacer"></span><span class="muted small">Absent & late alerts go to parents automatically.</span><button class="btn primary" id="save">${icon('check')} Save attendance</button></div></div>`;
    const state = {};
    const load = async () => {
      const d = await GET(`/api/attendance?class_id=${$('#cls', box).value}&date=${$('#dt', box).value}`);
      d.students.forEach((s) => { state[s.student_id] = s.status; });
      $('#list', box).innerHTML = table([
        { label: '#', render: (r, i) => i + 1 }, { label: 'Student', render: (r) => `<b>${esc(r.name)}</b><div class="muted small">${esc(r.admission_no)}</div>` },
        { label: 'Parent', key: 'parent_name' },
        { label: 'Status', render: (r) => `<div class="seg" data-id="${r.student_id}">${['present', 'absent', 'late'].map((s) => `<button type="button" data-s="${s}">${cap(s)}</button>`).join('')}</div>` },
      ], d.students, 'No students in this class.');
      paint();
    };
    const paint = () => {
      $$('.seg', box).forEach((g) => $$('button', g).forEach((b) => b.className = state[g.dataset.id] === b.dataset.s ? 'on-' + b.dataset.s : ''));
      const v = Object.values(state); const c = (s) => v.filter((x) => x === s).length;
      $('#summary', box).textContent = `${c('present')} present · ${c('absent')} absent · ${c('late')} late · ${c('')} unmarked`;
    };
    box.addEventListener('click', (e) => { const b = e.target.closest('.seg button'); if (b) { state[b.parentElement.dataset.id] = b.dataset.s; paint(); } });
    $('#allP', box).onclick = () => { Object.keys(state).forEach((k) => { if (!state[k]) state[k] = 'present'; }); paint(); };
    $('#cls', box).onchange = load; $('#dt', box).onchange = load;
    $('#save', box).onclick = async () => {
      const records = Object.entries(state).filter(([, s]) => s).map(([id, s]) => ({ student_id: Number(id), status: s }));
      if (!records.length) return toast('Mark at least one student', 'error');
      try { const r = await POST('/api/attendance', { class_id: Number($('#cls', box).value), date: $('#dt', box).value, records }); toast(`Saved ${r.saved} records · ${r.alerts_sent} parent alert(s) sent`, 'success'); }
      catch (err) { toast(err.message, 'error'); }
    };
    await load();
  }
  async function reportView() {
    box.innerHTML = `<div class="card"><div class="card-h">
      <select class="input" id="rc" style="width:auto">${classOptions('All classes').map(([v, l]) => `<option value="${v}">${esc(l)}</option>`).join('')}</select>
      <input class="input" type="date" id="rfrom" value="${addDays(-30)}" style="width:auto"><input class="input" type="date" id="rto" value="${todayStr()}" style="width:auto">
    </div><div id="rl">${loading()}</div></div>`;
    const load = async () => {
      const d = await GET(`/api/attendance/report?class_id=${$('#rc', box).value}&from=${$('#rfrom', box).value}&to=${$('#rto', box).value}`);
      $('#rl', box).innerHTML = table([
        { label: 'Student', render: (r) => `<b>${esc(r.name)}</b><div class="muted small">${esc(r.admission_no)}</div>` }, { label: 'Class', key: 'class_name' },
        { label: 'Present', key: 'present', num: true }, { label: 'Late', key: 'late', num: true }, { label: 'Absent', key: 'absent', num: true },
        { label: 'Attendance', num: true, render: (r) => r.total ? `<span class="badge ${r.percent >= 90 ? 'green' : r.percent >= 80 ? 'amber' : 'red'}">${r.percent}%</span>` : '<span class="muted">—</span>' },
      ], d.rows);
    };
    ['rc', 'rfrom', 'rto'].forEach((id) => $('#' + id, box).onchange = load);
    await load();
  }
  await markView();
};

/* ---------- Students ---------- */
PAGES.students = async (el) => {
  await Promise.all([loadClasses(), loadRoutes()]);
  el.innerHTML = `<div class="card"><div class="card-h">
    <input class="input" id="q" placeholder="Search name, admission no or parent…" style="max-width:280px">
    <select class="input" id="cf" style="width:auto">${classOptions('All classes').map(([v, l]) => `<option value="${v}">${esc(l)}</option>`).join('')}</select>
    <span class="spacer"></span>${isAdmin() ? `<button class="btn primary" id="add">${icon('plus')} Add student</button>` : ''}
  </div><div id="sl">${loading()}</div></div>`;
  let parents = [];
  const load = async () => {
    const rows = await GET(`/api/students?class_id=${$('#cf', el).value}&q=${encodeURIComponent($('#q', el).value)}`);
    $('#sl', el).innerHTML = table([
      { label: 'Adm. No', key: 'admission_no' },
      { label: 'Student', render: (r) => `<div class="row" style="flex-wrap:nowrap"><div class="avatar">${esc(initials(r.name))}</div><b>${esc(r.name)}</b></div>` },
      { label: 'Class', key: 'class_name' },
      { label: 'Parent', render: (r) => `${esc(r.parent_name || '—')}<div class="muted small">${esc(r.parent_phone || '')}</div>` },
      { label: 'Bus route', render: (r) => esc(r.route_name || '—') },
      { label: 'Attendance', num: true, render: (r) => r.attendance_pct == null ? '<span class="muted">—</span>' : `<span class="badge ${r.attendance_pct >= 90 ? 'green' : r.attendance_pct >= 80 ? 'amber' : 'red'}">${r.attendance_pct}%</span>` },
      { label: 'Fee due', num: true, render: (r) => r.fee_due ? `<b style="color:var(--danger)">${money(r.fee_due)}</b>` : '<span class="badge green">Clear</span>' },
      { label: '', render: (r) => `<div class="row" style="flex-wrap:nowrap;justify-content:flex-end">${r.parent_id ? `<a class="btn sm" href="#/messages/${r.parent_id}" title="Message parent">${icon('chat')}</a>` : ''}${isAdmin() ? `<button class="btn sm" data-edit="${r.id}">${icon('edit')}</button><button class="btn sm danger" data-del="${r.id}">${icon('trash')}</button>` : ''}</div>` },
    ], rows, 'No students found.');
    $$('[data-edit]', el).forEach((b) => b.onclick = () => edit(rows.find((r) => r.id == b.dataset.edit)));
    $$('[data-del]', el).forEach((b) => b.onclick = async () => {
      if (!(await confirmBox('Delete this student and all their attendance, fee and result records?', 'Delete'))) return;
      try { await DEL('/api/students/' + b.dataset.del); toast('Student deleted'); load(); } catch (e) { toast(e.message, 'error'); }
    });
  };
  async function edit(s = {}) {
    if (!parents.length) parents = await GET('/api/users?role=parent');
    modal({
      title: s.id ? 'Edit student' : 'Add student', wide: true,
      body: formFields([
        { name: 'name', label: 'Full name', required: true }, { name: 'admission_no', label: 'Admission no.', placeholder: 'Auto if empty' },
        { name: 'class_id', label: 'Class', type: 'select', required: true, options: classOptions() },
        { name: 'gender', label: 'Gender', type: 'select', options: [['', '—'], ['M', 'Male'], ['F', 'Female']] },
        { name: 'dob', label: 'Date of birth', type: 'date' },
        { name: 'route_id', label: 'Bus route', type: 'select', options: [[0, 'No bus'], ...S.routes.map((r) => [r.id, r.name])] },
        { name: 'parent_id', label: 'Parent account', type: 'select', full: true, options: [[0, '➕ Create a new parent account below'], ...parents.map((p) => [p.id, `${p.name} · ${p.email}`])] },
      ], s) + `<div id="newParent" class="form-grid" style="${s.parent_id ? 'display:none' : ''}">
        ${field({ name: 'parent_name', label: 'Parent name' })}${field({ name: 'parent_phone', label: 'Parent mobile (WhatsApp)', placeholder: '07X XXX XXXX' })}
        ${field({ name: 'parent_email', label: 'Parent login email' })}${field({ name: 'parent_password', label: 'Parent password', hint: 'Share this with the parent so they can sign in to the app.' })}
      </div>`,
      onOpen: (f) => { f.elements.parent_id.onchange = () => { $('#newParent', f).style.display = f.elements.parent_id.value == 0 ? '' : 'none'; }; },
      onSubmit: async (v) => {
        const body = { ...v, class_id: Number(v.class_id), route_id: Number(v.route_id), parent_id: Number(v.parent_id) };
        if (s.id) await PUT('/api/students/' + s.id, body); else await POST('/api/students', body);
        toast('Student saved', 'success'); parents = []; load();
      },
    });
  }
  let t; $('#q', el).oninput = () => { clearTimeout(t); t = setTimeout(load, 250); };
  $('#cf', el).onchange = load;
  const add = $('#add', el); add && (add.onclick = () => edit());
  await load();
};

/* ---------- Fees ---------- */
PAGES.fees = async (el) => {
  await loadClasses();
  el.innerHTML = `<div class="card"><div class="card-h">
    <div class="tabs" id="ft">${[['', 'All'], ['unpaid', 'Unpaid'], ['overdue', 'Overdue'], ['paid', 'Paid']].map(([v, l], i) => `<button data-v="${v}" class="${i === 1 ? 'active' : ''}">${l}</button>`).join('')}</div>
    <select class="input" id="fc" style="width:auto">${classOptions('All classes').map(([v, l]) => `<option value="${v}">${esc(l)}</option>`).join('')}</select>
    <span class="spacer"></span>${isAdmin() ? `<button class="btn" id="remind">${icon('bell')} Send due reminders</button><button class="btn primary" id="issue">${icon('plus')} Issue fee</button>` : ''}
  </div><div id="fsum" class="card-b row" style="border-bottom:1px solid var(--border)"></div><div id="fl">${loading()}</div></div>`;
  let status = 'unpaid';
  const load = async () => {
    const rows = await GET(`/api/fees?status=${status}&class_id=${$('#fc', el).value}`);
    const sum = (f) => rows.filter(f).reduce((a, r) => a + r.amount, 0);
    $('#fsum', el).innerHTML = `<span>Showing <b>${rows.length}</b> fee(s)</span><span class="badge amber">Unpaid ${money(sum((r) => r.status === 'unpaid'))}</span><span class="badge red">Overdue ${money(sum((r) => r.overdue))}</span><span class="badge green">Paid ${money(sum((r) => r.status === 'paid'))}</span>`;
    $('#fl', el).innerHTML = table([
      { label: 'Student', render: (r) => `<b>${esc(r.student_name)}</b><div class="muted small">${esc(r.admission_no)} · ${esc(r.class_name)}</div>` },
      { label: 'Fee', key: 'title' }, { label: 'Amount', num: true, render: (r) => `<b>${money(r.amount)}</b>` },
      { label: 'Due', render: (r) => fmtDate(r.due_date) },
      { label: 'Status', render: (r) => r.overdue ? badge('overdue') : badge(r.status) + (r.paid_at ? `<div class="muted small">${fmtDate(r.paid_at)}</div>` : '') },
      { label: 'Last reminder', render: (r) => r.last_reminded ? fmtDate(r.last_reminded) : '<span class="muted">—</span>' },
      { label: '', render: (r) => isAdmin() ? `<div class="row" style="flex-wrap:nowrap;justify-content:flex-end">${r.status === 'unpaid' ? `<button class="btn sm" data-remind="${r.id}" title="Send reminder">${icon('bell')}</button><button class="btn sm success" data-pay="${r.id}">${icon('check')} Paid</button>` : ''}<button class="btn sm danger" data-del="${r.id}">${icon('trash')}</button></div>` : '' },
    ], rows, 'No fees match this filter.');
    $$('[data-pay]', el).forEach((b) => b.onclick = async () => { if (!(await confirmBox('Mark this fee as paid? The parent will get a payment receipt notification.', 'Mark paid'))) return; await POST(`/api/fees/${b.dataset.pay}/pay`).then(() => { toast('Payment recorded', 'success'); load(); }).catch((e) => toast(e.message, 'error')); });
    $$('[data-remind]', el).forEach((b) => b.onclick = async () => { await POST(`/api/fees/${b.dataset.remind}/remind`).then(() => { toast('Reminder sent', 'success'); load(); }).catch((e) => toast(e.message, 'error')); });
    $$('[data-del]', el).forEach((b) => b.onclick = async () => { if (!(await confirmBox('Delete this fee record?', 'Delete'))) return; await DEL('/api/fees/' + b.dataset.del).then(load).catch((e) => toast(e.message, 'error')); });
  };
  $$('#ft button', el).forEach((b) => b.onclick = () => { status = b.dataset.v; $$('#ft button', el).forEach((x) => x.classList.toggle('active', x === b)); load(); });
  $('#fc', el).onchange = load;
  const r = $('#remind', el); r && (r.onclick = async () => { await sendDueReminders(); load(); });
  const issue = $('#issue', el);
  issue && (issue.onclick = () => modal({
    title: 'Issue fee',
    body: formFields([
      { name: 'class_id', label: 'Students', type: 'select', full: true, options: classOptions('All students in school') },
      { name: 'title', label: 'Fee title', required: true, placeholder: 'Term 3 Facility Fee', full: true },
      { name: 'amount', label: 'Amount (Rs.)', type: 'number', required: true, min: 1, step: '0.01' },
      { name: 'due_date', label: 'Due date', type: 'date', required: true, value: addDays(14) },
      { name: 'notify', label: 'Notify parents now (uses Fee channels from Settings)', type: 'checkbox', value: true, full: true },
    ]),
    submit: 'Issue fee',
    onSubmit: async (v) => { const res = await POST('/api/fees', { ...v, class_id: Number(v.class_id) }); toast(`Issued to ${res.created} student(s), ${res.notified} parent(s) notified`, 'success'); load(); },
  }));
  await load();
};

/* ---------- Homework ---------- */
PAGES.homework = async (el) => {
  await loadClasses();
  el.innerHTML = `<div class="card"><div class="card-h"><select class="input" id="hc" style="width:auto">${classOptions('All classes').map(([v, l]) => `<option value="${v}">${esc(l)}</option>`).join('')}</select><span class="spacer"></span><button class="btn primary" id="add">${icon('plus')} Assign homework</button></div><div id="hl">${loading()}</div></div>`;
  const load = async () => {
    const rows = await GET('/api/homework?class_id=' + $('#hc', el).value);
    $('#hl', el).innerHTML = rows.length ? rows.map((h) => `<div class="list-item">${catIcon('homework')}<div class="grow"><div class="t1">${esc(h.subject)} · ${esc(h.title)}</div><div class="t2">${esc(h.class_name)} · Due ${fmtDate(h.due_date)} · by ${esc(h.teacher_name || '—')}</div>${h.description ? `<div class="small" style="margin-top:4px">${esc(h.description)}</div>` : ''}</div>${h.due_date < todayStr() ? badge('', 'Closed') : badge('present', 'Open')}<button class="btn sm danger" data-del="${h.id}">${icon('trash')}</button></div>`).join('') : '<div class="empty">No homework assigned.</div>';
    $$('[data-del]', el).forEach((b) => b.onclick = async () => { if (await confirmBox('Delete this homework?', 'Delete')) DEL('/api/homework/' + b.dataset.del).then(load).catch((e) => toast(e.message, 'error')); });
  };
  $('#hc', el).onchange = load;
  $('#add', el).onclick = () => modal({
    title: 'Assign homework', submit: 'Assign',
    body: formFields([
      { name: 'class_id', label: 'Class', type: 'select', required: true, options: classOptions() },
      { name: 'subject', label: 'Subject', required: true, placeholder: 'Mathematics' },
      { name: 'title', label: 'Title', required: true, full: true, placeholder: 'Complete Exercise 5.2' },
      { name: 'description', label: 'Instructions', type: 'textarea', full: true, rows: 3 },
      { name: 'due_date', label: 'Due date', type: 'date', required: true, value: addDays(1) },
      { name: 'notify', label: 'Notify parents & students', type: 'checkbox', value: true },
    ]),
    onSubmit: async (v) => { const r = await POST('/api/homework', { ...v, class_id: Number(v.class_id) }); toast(`Homework assigned · ${r.notified} notified`, 'success'); load(); },
  });
  await load();
};

/* ---------- Exams ---------- */
PAGES.exams = async (el) => {
  await loadClasses();
  el.innerHTML = `<div class="card"><div class="card-h"><select class="input" id="ec" style="width:auto">${classOptions('All classes').map(([v, l]) => `<option value="${v}">${esc(l)}</option>`).join('')}</select><span class="spacer"></span><button class="btn primary" id="add">${icon('plus')} Schedule exam</button></div><div id="el">${loading()}</div></div>`;
  const load = async () => {
    const rows = await GET('/api/exams?class_id=' + $('#ec', el).value);
    $('#el', el).innerHTML = table([
      { label: 'Exam', render: (r) => `<b>${esc(r.subject)}</b> – ${esc(r.title)}` }, { label: 'Class', key: 'class_name' },
      { label: 'Date', render: (r) => `${fmtDate(r.exam_date)}${r.exam_time ? ' · ' + esc(r.exam_time) : ''}` },
      { label: 'Max', key: 'max_marks', num: true }, { label: 'Results', num: true, render: (r) => `${r.result_count}${r.average != null ? ` <span class="muted small">(avg ${r.average})</span>` : ''}` },
      { label: 'Status', render: (r) => r.published ? badge('present', 'Published') : r.exam_date >= todayStr() ? badge('', 'Upcoming') : badge('late', 'Awaiting results') },
      { label: '', render: (r) => `<div class="row" style="flex-wrap:nowrap;justify-content:flex-end"><button class="btn sm" data-res="${r.id}">${icon('edit')} Results</button><button class="btn sm danger" data-del="${r.id}">${icon('trash')}</button></div>` },
    ], rows, 'No exams scheduled.');
    $$('[data-res]', el).forEach((b) => b.onclick = () => results(b.dataset.res));
    $$('[data-del]', el).forEach((b) => b.onclick = async () => { if (await confirmBox('Delete this exam and its results?', 'Delete')) DEL('/api/exams/' + b.dataset.del).then(load).catch((e) => toast(e.message, 'error')); });
  };
  async function results(id) {
    const d = await GET(`/api/exams/${id}/results`);
    modal({
      title: `${d.exam.subject} – ${d.exam.title} (${d.exam.class_name})`, wide: true, submit: 'Save results',
      body: `<p class="muted small" style="margin-top:0">Max marks: <b>${d.exam.max_marks}</b>. Leave blank for students who did not sit.</p>` + table([
        { label: 'Student', render: (r) => `<b>${esc(r.name)}</b><div class="muted small">${esc(r.admission_no)}</div>` },
        { label: 'Marks', render: (r) => `<input class="input" type="number" min="0" max="${d.exam.max_marks}" step="0.5" name="m_${r.student_id}" value="${r.marks ?? ''}" style="width:100px">` },
        { label: 'Remarks', render: (r) => `<input class="input" name="r_${r.student_id}" value="${esc(r.remarks)}">` },
      ], d.students) + `<div class="field mt"><label class="check"><input type="checkbox" name="publish" ${d.exam.published ? '' : 'checked'}> Publish and send results to parents now</label></div>`,
      onSubmit: async (v) => {
        const records = d.students.map((s) => ({ student_id: s.student_id, marks: v['m_' + s.student_id], remarks: v['r_' + s.student_id] || '' }));
        const r = await POST(`/api/exams/${id}/results`, { publish: v.publish, records });
        toast(`Results saved${v.publish ? ` · ${r.notified} notified` : ''}`, 'success'); load();
      },
    });
  }
  $('#ec', el).onchange = load;
  $('#add', el).onclick = () => modal({
    title: 'Schedule exam', submit: 'Schedule',
    body: formFields([
      { name: 'class_id', label: 'Class', type: 'select', required: true, options: classOptions() },
      { name: 'subject', label: 'Subject', required: true }, { name: 'title', label: 'Exam title', required: true, placeholder: 'Unit Test' },
      { name: 'max_marks', label: 'Max marks', type: 'number', value: 100, min: 1 },
      { name: 'exam_date', label: 'Date', type: 'date', required: true, value: addDays(7) }, { name: 'exam_time', label: 'Time', type: 'time', value: '09:00' },
      { name: 'notify', label: 'Send exam notice to parents & students', type: 'checkbox', value: true, full: true },
    ]),
    onSubmit: async (v) => { const r = await POST('/api/exams', { ...v, class_id: Number(v.class_id) }); toast(`Exam scheduled · ${r.notified} notified`, 'success'); load(); },
  });
  await load();
};

/* ---------- Transport ---------- */
PAGES.transport = async (el) => {
  const [routes, updates] = await Promise.all([loadRoutes(true), GET('/api/bus-updates')]);
  el.innerHTML = `<div class="stack">
    <div class="row">${isAdmin() ? `<button class="btn primary" id="addR">${icon('plus')} Add route</button>` : ''}</div>
    <div class="grid g-3">${routes.map((r) => `<div class="card"><div class="card-h">${catIcon('transport')}<div><h3>${esc(r.name)}</h3><div class="muted small">Bus ${esc(r.bus_no || '—')} · ${r.student_count} students</div></div></div>
      <div class="card-b"><div class="small">Driver: <b>${esc(r.driver_name || '—')}</b> ${r.driver_phone ? '· ' + esc(r.driver_phone) : ''}</div>
      ${r.last_update ? `<div class="preview-phone mt small">${esc(r.last_update)}<div class="muted small">${ago(r.last_update_at)}</div></div>` : '<div class="muted small mt">No updates yet.</div>'}
      <div class="row mt"><button class="btn primary sm" data-up="${r.id}">${icon('send')} Post update</button>${isAdmin() ? `<button class="btn sm" data-edit="${r.id}">${icon('edit')}</button><button class="btn sm danger" data-del="${r.id}">${icon('trash')}</button>` : ''}</div></div></div>`).join('') || '<div class="card empty">No routes yet.</div>'}</div>
    <div class="card"><div class="card-h"><h3>Recent bus updates</h3></div>${updates.length ? updates.map((u) => `<div class="list-item">${catIcon('transport')}<div class="grow"><div class="t1">${esc(u.route_name)} ${u.bus_no ? `<span class="badge">${esc(u.bus_no)}</span>` : ''}</div><div class="t2">${esc(u.message)}</div></div><span class="muted small">${ago(u.created_at)}</span></div>`).join('') : '<div class="empty">No updates sent.</div>'}</div>
  </div>`;
  const quick = ['Running 10 mins late due to traffic.', 'Has departed from school.', 'Has reached school safely.', 'Is delayed due to a breakdown. A replacement bus is on the way.', 'Will not operate tomorrow.'];
  $$('[data-up]', el).forEach((b) => b.onclick = () => {
    const r = routes.find((x) => x.id == b.dataset.up);
    modal({
      title: 'Bus update – ' + r.name, submit: 'Send to parents',
      body: `<div class="suggests" style="margin-bottom:12px">${quick.map((q) => `<button type="button">${esc(q)}</button>`).join('')}</div>` + field({ name: 'message', label: 'Message', type: 'textarea', required: true, rows: 3, value: `Bus No. ${r.bus_no || ''} ` }),
      onOpen: (f) => $$('.suggests button', f).forEach((q) => q.onclick = () => { f.elements.message.value = `Bus No. ${r.bus_no || ''} ${q.textContent}`; }),
      onSubmit: async (v) => { const res = await POST(`/api/routes/${r.id}/updates`, v); toast(`Update sent to ${res.notified} parent(s)`, 'success'); PAGES.transport(el); },
    });
  });
  const edit = (r = {}) => modal({
    title: r.id ? 'Edit route' : 'Add route',
    body: formFields([{ name: 'name', label: 'Route name', required: true, full: true }, { name: 'bus_no', label: 'Bus number' }, { name: 'driver_name', label: 'Driver name' }, { name: 'driver_phone', label: 'Driver phone' }], r),
    onSubmit: async (v) => { r.id ? await PUT('/api/routes/' + r.id, v) : await POST('/api/routes', v); toast('Route saved', 'success'); PAGES.transport(el); },
  });
  const add = $('#addR', el); add && (add.onclick = () => edit());
  $$('[data-edit]', el).forEach((b) => b.onclick = () => edit(routes.find((x) => x.id == b.dataset.edit)));
  $$('[data-del]', el).forEach((b) => b.onclick = async () => { if (await confirmBox('Delete this route? Students will be unassigned.', 'Delete')) DEL('/api/routes/' + b.dataset.del).then(() => PAGES.transport(el)).catch((e) => toast(e.message, 'error')); });
};

/* ---------- Users ---------- */
PAGES.users = async (el) => {
  el.innerHTML = `<div class="card"><div class="card-h"><div class="tabs" id="ut">${[['', 'All'], ['admin', 'Admins'], ['teacher', 'Teachers'], ['parent', 'Parents'], ['student', 'Students']].map(([v, l], i) => `<button data-v="${v}" class="${i === 0 ? 'active' : ''}">${l}</button>`).join('')}</div><span class="spacer"></span><button class="btn primary" id="add">${icon('plus')} Add user</button></div><div id="ul">${loading()}</div></div>`;
  let role = '';
  const load = async () => {
    const rows = await GET('/api/users?role=' + role);
    $('#ul', el).innerHTML = table([
      { label: 'Name', render: (r) => `<div class="row" style="flex-wrap:nowrap"><div class="avatar">${esc(initials(r.name))}</div><div><b>${esc(r.name)}</b><div class="muted small">${esc(r.email)}</div></div></div>` },
      { label: 'Role', render: (r) => badge('', cap(r.role)) }, { label: 'Mobile', key: 'phone' }, { label: 'WhatsApp', key: 'whatsapp' },
      { label: 'Children', render: (r) => esc(r.children || '') }, { label: 'Language', key: 'language' },
      { label: 'Status', render: (r) => r.active ? badge('present', 'Active') : badge('absent', 'Disabled') },
      { label: '', render: (r) => `<div class="row" style="flex-wrap:nowrap;justify-content:flex-end"><button class="btn sm" data-edit="${r.id}">${icon('edit')}</button>${r.id !== S.me.id ? `<button class="btn sm danger" data-del="${r.id}">${icon('trash')}</button>` : ''}</div>` },
    ], rows);
    $$('[data-edit]', el).forEach((b) => b.onclick = () => edit(rows.find((r) => r.id == b.dataset.edit)));
    $$('[data-del]', el).forEach((b) => b.onclick = async () => { if (await confirmBox('Delete this user permanently?', 'Delete')) DEL('/api/users/' + b.dataset.del).then(load).catch((e) => toast(e.message, 'error')); });
  };
  const edit = (u = {}) => modal({
    title: u.id ? 'Edit user' : 'Add user',
    body: formFields([
      { name: 'name', label: 'Full name', required: true }, { name: 'email', label: 'Login email', type: 'email', required: true },
      { name: 'role', label: 'Role', type: 'select', options: [['parent', 'Parent'], ['teacher', 'Teacher'], ['admin', 'Administrator'], ['student', 'Student']], value: u.role || role || 'parent' },
      { name: 'language', label: 'Language', type: 'select', options: ['English', 'Sinhala', 'Tamil'] },
      { name: 'phone', label: 'Mobile', placeholder: '07X XXX XXXX' }, { name: 'whatsapp', label: 'WhatsApp', placeholder: 'Same as mobile if empty' },
      { name: 'password', label: u.id ? 'New password (leave blank to keep)' : 'Password', type: 'text', required: !u.id },
      ...(u.id ? [{ name: 'active', label: 'Account active', type: 'checkbox', value: !!u.active }] : []),
    ], u),
    onSubmit: async (v) => { u.id ? await PUT('/api/users/' + u.id, v) : await POST('/api/users', v); toast('User saved', 'success'); load(); },
  });
  $$('#ut button', el).forEach((b) => b.onclick = () => { role = b.dataset.v; $$('#ut button', el).forEach((x) => x.classList.toggle('active', x === b)); load(); });
  $('#add', el).onclick = () => edit();
  await load();
};

/* ---------- Classes ---------- */
PAGES.classes = async (el) => {
  const [rows, teachers] = await Promise.all([loadClasses(true), GET('/api/users?role=teacher')]);
  el.innerHTML = `<div class="card"><div class="card-h"><h3>Classes</h3><span class="spacer"></span><button class="btn primary" id="add">${icon('plus')} Add class</button></div>${table([
    { label: 'Class', render: (r) => `<b>${esc(r.label)}</b>` }, { label: 'Class teacher', render: (r) => esc(r.teacher_name || '—') },
    { label: 'Students', key: 'student_count', num: true },
    { label: '', render: (r) => `<div class="row" style="justify-content:flex-end"><button class="btn sm" data-edit="${r.id}">${icon('edit')}</button><button class="btn sm danger" data-del="${r.id}">${icon('trash')}</button></div>` },
  ], rows, 'No classes yet.')}</div>`;
  const edit = (c = {}) => modal({
    title: c.id ? 'Edit class' : 'Add class',
    body: formFields([{ name: 'name', label: 'Grade / class name', required: true, placeholder: 'Grade 6' }, { name: 'section', label: 'Section', placeholder: 'A' },
      { name: 'teacher_id', label: 'Class teacher', type: 'select', full: true, options: [[0, '—'], ...teachers.map((t) => [t.id, t.name])] }], c),
    onSubmit: async (v) => { const b = { ...v, teacher_id: Number(v.teacher_id) }; c.id ? await PUT('/api/classes/' + c.id, b) : await POST('/api/classes', b); toast('Class saved', 'success'); PAGES.classes(el); },
  });
  $('#add', el).onclick = () => edit();
  $$('[data-edit]', el).forEach((b) => b.onclick = () => edit(rows.find((r) => r.id == b.dataset.edit)));
  $$('[data-del]', el).forEach((b) => b.onclick = async () => { if (await confirmBox('Delete this class? Its homework and exams will also be removed.', 'Delete')) DEL('/api/classes/' + b.dataset.del).then(() => PAGES.classes(el)).catch((e) => toast(e.message, 'error')); });
};

/* ---------- Settings ---------- */
PAGES.settings = async (el) => {
  const s = await GET('/api/settings');
  const cats = [['attendance', 'Attendance alerts'], ['fees', 'Fee reminders & receipts'], ['homework', 'Homework'], ['exams', 'Exam notices'], ['results', 'Results'], ['transport', 'Bus updates'], ['messages', 'New chat message']];
  el.innerHTML = `<form id="sf" class="stack">
    <div class="grid g-2">
      <div class="card"><div class="card-h"><h3>School profile</h3></div><div class="card-b">${formFields([
        { name: 'school_name', label: 'School name', full: true }, { name: 'school_phone', label: 'Phone' }, { name: 'school_email', label: 'Email' },
        { name: 'school_address', label: 'Address', full: true }], s)}</div></div>
      <div class="card"><div class="card-h"><h3>Integrations</h3></div><div class="card-b">
        <div class="list-item" style="padding:8px 0">${catIcon('message')}<div class="grow"><div class="t1">SMS</div><div class="t2">${esc(s.providers.sms)}</div></div></div>
        <div class="list-item" style="padding:8px 0"><div class="ic tint-green">${icon('whatsapp')}</div><div class="grow"><div class="t1">WhatsApp</div><div class="t2">${esc(s.providers.whatsapp)}</div></div></div>
        <div class="list-item" style="padding:8px 0"><div class="ic tint-violet">${icon('sparkle')}</div><div class="grow"><div class="t1">AI (Claude)</div><div class="t2">${esc(s.providers.ai)}</div></div></div>
        <p class="muted small">Providers and API keys are configured in the server's <code>.env</code> file (see README). "Simulation" mode logs messages without sending them.</p>
        <div class="row"><select class="input" id="tch" style="width:auto"><option value="sms">SMS</option><option value="whatsapp">WhatsApp</option></select><input class="input" id="tto" placeholder="07X XXX XXXX" style="flex:1;min-width:140px"><button type="button" class="btn" id="tsend">Send test</button></div>
      </div></div>
    </div>
    <div class="card"><div class="card-h"><h3>Automated notifications</h3><span class="muted small">Choose which channels each automatic alert uses</span></div>
      ${table([{ label: 'Alert', render: (r) => `<b>${r[1]}</b>` }, ...['app', 'sms', 'whatsapp'].map((c) => ({ label: CH[c].label, render: (r) => `<input type="checkbox" style="width:18px;height:18px;accent-color:var(--brand)" data-group="channels_${r[0]}" name="c_${r[0]}_${c}" value="${c}" ${(s['channels_' + r[0]] || '').split(',').includes(c) ? 'checked' : ''}>` }))], cats)}
      <div class="card-b form-grid" style="border-top:1px solid var(--border)">
        ${field({ name: 'notify_present', label: 'Also notify parents when a child is marked Present', type: 'checkbox', value: s.notify_present === '1', full: true })}
        ${field({ name: 'fee_reminder_days', label: 'Remind about fees due within (days)', type: 'number', min: 0, value: s.fee_reminder_days })}
        ${field({ name: 'fee_reminder_hour', label: 'Daily reminder time (hour, 0–23)', type: 'number', min: 0, value: s.fee_reminder_hour })}
      </div></div>
    <div><button class="btn primary">${icon('check')} Save settings</button></div>
  </form>
  <div id="dbcard" class="stack" style="margin-top:18px"></div>`;
  renderStorage($('#dbcard', el));
  $('#tsend', el).onclick = async () => {
    try { const r = await POST('/api/settings/test-message', { channel: $('#tch', el).value, to: $('#tto', el).value }); toast('Test result: ' + r.status, 'success'); }
    catch (e) { toast(e.message, 'error'); }
  };
  $('#sf', el).addEventListener('submit', async (e) => {
    e.preventDefault();
    const v = readForm(e.target), body = {};
    ['school_name', 'school_phone', 'school_email', 'school_address'].forEach((k) => body[k] = v[k]);
    cats.forEach(([c]) => body['channels_' + c] = (v['channels_' + c] || []).join(','));
    body.notify_present = v.notify_present ? '1' : '0';
    body.fee_reminder_days = String(v.fee_reminder_days ?? 3); body.fee_reminder_hour = String(v.fee_reminder_hour ?? 9);
    try { await PUT('/api/settings', body); S.school.name = v.school_name; toast('Settings saved', 'success'); } catch (err) { toast(err.message, 'error'); }
  });
};

/* ---------- Database & backups (inside Settings) ---------- */
const fileSize = (n) => n >= 1048576 ? (n / 1048576).toFixed(1) + ' MB' : Math.max(1, Math.round(n / 1024)) + ' KB';

async function renderStorage(el) {
  const st = await GET('/api/system/storage');
  const locked = st.db_source === 'env';
  const source = { env: 'set by DB_PATH in the server environment', settings: 'chosen in Settings', default: 'default location' }[st.db_source];
  el.innerHTML = `<div class="card"><div class="card-h"><h3>Database</h3><span class="muted small">Where the school's data is stored on the server</span></div><div class="card-b stack">
      ${st.notice ? `<div class="list-item" style="padding:8px 0">${catIcon('announcement')}<div class="grow"><div class="t2">${esc(st.notice)}</div></div></div>` : ''}
      <div class="list-item" style="padding:8px 0"><div class="ic tint-blue">${icon('layers')}</div><div class="grow"><div class="t1"><code>${esc(st.db_path)}</code></div><div class="t2">${fileSize(st.db_size)} · ${esc(source)}</div></div></div>
      ${locked ? `<p class="muted small">Remove <code>DB_PATH</code> from the server's environment or <code>.env</code> file to change the location here.</p>` : `
      <form id="dbf" class="form-grid">
        ${field({ name: 'path', label: 'New database file path', full: true, required: true, placeholder: '/srv/epoch/epoch.db' })}
        ${field({ name: 'mode', label: 'What to do', type: 'select', full: true, options: [['copy', 'Copy the current data to the new file and use it'], ['open', 'Open a database file that is already there (e.g. a backup)']] }, 'copy')}
        <p class="muted small" style="grid-column:1/-1">The system restarts for a few seconds to switch files. The current file is left untouched. The location is remembered in <code>${esc(st.config_file)}</code>.</p>
        <div><button class="btn">${icon('check')} Change database location</button></div>
      </form>`}
    </div></div>
    <div class="card"><div class="card-h"><h3>Backups</h3><span class="spacer"></span>
      <a class="btn" href="/api/system/backup/download">${icon('refresh')} Download backup</a>
      <button type="button" class="btn primary" id="bnow">${icon('check')} Back up now</button></div>
      <form id="bf" class="card-b form-grid">
        ${field({ name: 'backup_dir', label: 'Backup folder (leave blank for a "backups" folder next to the database)', full: true, placeholder: st.backup_dir }, st.backup_dir_custom ? st.backup_dir : '')}
        ${field({ name: 'backup_daily', label: 'Make a backup automatically every day', type: 'checkbox' }, st.backup_daily)}
        ${field({ name: 'backup_keep', label: 'Backups to keep (0 = keep all)', type: 'number', min: 0 }, st.backup_keep)}
        <div><button class="btn">${icon('check')} Save backup settings</button></div>
      </form>
      ${table([{ label: 'Backup file', render: (r) => `<code>${esc(r.name)}</code>` }, { label: 'Size', render: (r) => fileSize(r.size), num: true }, { label: 'Created', render: (r) => esc(r.created) }],
        st.backups, `No backups yet in ${st.backup_dir}`)}
      <div class="card-b"><p class="muted small">Backups are complete, consistent copies made while the system is running. ${st.last_backup ? 'Last backup: ' + esc(st.last_backup) + '.' : ''} To restore one, use "Open a database file that is already there" above with the backup's path.</p></div>
    </div>`;

  $('#bnow', el).onclick = async (e) => {
    e.target.disabled = true;
    try { const r = await POST('/api/system/backup'); toast('Backup saved to ' + r.path, 'success'); renderStorage(el); }
    catch (err) { toast(err.message, 'error'); e.target.disabled = false; }
  };
  $('#bf', el).addEventListener('submit', async (e) => {
    e.preventDefault();
    const v = readForm(e.target);
    try { await PUT('/api/system/storage', { backup_dir: v.backup_dir || '', backup_daily: !!v.backup_daily, backup_keep: Number(v.backup_keep || 0) }); toast('Backup settings saved', 'success'); renderStorage(el); }
    catch (err) { toast(err.message, 'error'); }
  });
  const dbf = $('#dbf', el);
  if (dbf) dbf.addEventListener('submit', async (e) => {
    e.preventDefault();
    const v = readForm(e.target);
    const msg = v.mode === 'open'
      ? `Switch to the database at ${v.path}? Everyone will be using that file's data from now on, and you may need to sign in again.`
      : `Copy all data to ${v.path} and use it from now on?`;
    if (!(await confirmBox(msg, 'Change location'))) return;
    try {
      await POST('/api/system/database', v);
      toast('Switching database, the system is restarting…');
      await waitForServer();
      location.reload();
    } catch (err) { toast(err.message, 'error'); }
  });
}

// waitForServer polls until the server answers again after a restart.
async function waitForServer() {
  await new Promise((r) => setTimeout(r, 1500));
  for (let i = 0; i < 40; i++) {
    try { const res = await fetch('/api/settings', { credentials: 'same-origin' }); if (res.status < 500) return; } catch { /* still restarting */ }
    await new Promise((r) => setTimeout(r, 500));
  }
}
