/* Parent & student mobile-style portal */
'use strict';

const P = { childId: null, data: null };
const TILES = [
  ['attendance', 'Attendance', 'calendar', 'tint-blue'], ['fees', 'Fees', 'money', 'tint-red'], ['homework', 'Homework', 'book', 'tint-orange'],
  ['exams', 'Exams', 'exam', 'tint-violet'], ['transport', 'Transport', 'bus', 'tint-orange'], ['alerts', 'Notices', 'megaphone', 'tint-green'],
  ['messages', 'Messages', 'chat', 'tint-blue'], ['results', 'Results', 'chart', 'tint-red'], ['timetable', 'Timetable', 'diary', 'tint-green'],
  ['progress', 'Progress', 'trophy', 'tint-orange'], ['pickup', 'Pickup', 'user', 'tint-teal'],
  ['ai', 'Ask AI', 'sparkle', 'tint-violet'],
];

async function renderPortal() {
  const view = (location.hash.slice(2) || 'home').split('/')[0];
  if (!S.children.length) {
    $('#app').innerHTML = `<div class="portal"><div class="p-head"><div class="p-top"><img src="assets/logo-mark.png" alt=""><b>${esc(S.school.name)}</b><span class="right"></span><button class="icon-btn" onclick="logout()">${icon('logout')}</button></div></div><div class="p-body"><div class="card empty">No student is linked to your account yet. Please contact the school office.</div></div></div>`;
    return;
  }
  try { P.childId = Number(localStorage.getItem('child')) || null; } catch {}
  if (!S.children.some((c) => c.id === P.childId)) P.childId = S.children[0].id;
  if (!P.data || P.data.student.id !== P.childId || view === 'home') P.data = await GET('/api/portal/' + P.childId);
  const d = P.data, st = d.student, isParent = S.me.role === 'parent';
  const unreadAlerts = d.notifications.filter((n) => !n.is_read).length;
  const dueFees = d.fees.filter((f) => f.status === 'unpaid').length;
  const nav = [['home', 'Home', 'home'], ['homework', 'Diary', 'diary'], ['alerts', 'Alerts', 'bell'], ['messages', 'Messages', 'chat'], ['profile', 'Profile', 'user']];

  $('#app').innerHTML = `<div class="portal">
    ${view === 'home' ? `<div class="p-head">
      <div class="p-top"><img src="assets/logo-mark.png" alt=""><b>${esc(S.school.name)}</b><span class="right"></span>
        <button class="icon-btn" id="themeBtn" aria-label="Toggle dark mode">${icon('moon')}</button></div>
      <div class="p-greet">${greeting()}<h2>${esc(isParent ? 'Parent' : st.name.split(' ')[0])}</h2></div>
    </div>
    <div class="card child-card"><div class="avatar">${esc(initials(st.name))}</div><div style="min-width:0;flex:1">
      ${S.children.length > 1 ? `<select id="childSel">${S.children.map((c) => `<option value="${c.id}" ${c.id === P.childId ? 'selected' : ''}>${esc(c.name)}</option>`).join('')}</select>` : `<b style="font-size:16px">${esc(st.name)}</b>`}
      <div class="muted small">${esc(st.class_name || '')} · ${esc(st.admission_no)}</div>
      <div class="mt" style="margin-top:6px">${d.today_status ? `<span class="status-pill badge ${STATUS_BADGE[d.today_status]}">${t('Today')}: ${t(cap(d.today_status))}</span>` : '<span class="status-pill badge">Today: not marked yet</span>'}</div>
    </div></div>` : `<div class="p-head" style="padding-bottom:18px;border-radius:0 0 20px 20px"><div class="p-top">
      <button class="icon-btn" onclick="location.hash='#/home'" aria-label="Back">${icon('home')}</button><b style="font-size:17px">${esc((TILES.find((t) => t[0] === view) || nav.find((n) => n[0] === view) || [, 'Home'])[1])}</b><span class="right muted small" style="color:#c6d3f5">${esc(st.name)}</span></div></div>`}
    <div class="p-body" id="pv"></div>
    ${S.aiEnabled && view !== 'ai' ? `<button class="fab" onclick="location.hash='#/ai'" title="Ask Epoch AI" aria-label="Ask Epoch AI">${icon('sparkle')}</button>` : ''}
    <nav class="bottom-nav">${nav.map(([id, label, ic]) => `<button class="${view === id || (id === 'homework' && view === 'homework') ? 'active' : ''}" onclick="location.hash='#/${id}'">${icon(ic)}${label}${id === 'alerts' && unreadAlerts ? `<span class="dot">${unreadAlerts}</span>` : ''}${id === 'messages' && S.unreadMessages ? `<span class="dot">${S.unreadMessages}</span>` : ''}</button>`).join('')}</nav>
  </div>`;
  const th = $('#themeBtn'); th && (th.onclick = toggleTheme);
  const sel = $('#childSel'); sel && (sel.onchange = () => { try { localStorage.setItem('child', sel.value); } catch {} P.data = null; renderPortal(); });
  const pv = $('#pv');
  (PV[view] || PV.home)(pv, d, { dueFees, unreadAlerts });
  window.scrollTo(0, 0);
}

const PV = {};
const sectionCard = (title, inner, extra = '') => `<div class="card"><div class="card-h"><h3>${esc(title)}</h3><span class="spacer"></span>${extra}</div>${inner}</div>`;

PV.home = (el, d, c) => {
  const latest = d.notifications.slice(0, 4);
  el.innerHTML = `<div class="tiles">${TILES.filter((t) => t[0] !== 'ai' || S.aiEnabled).map(([id, label, ic, tint]) => {
    const dot = id === 'fees' ? c.dueFees : id === 'alerts' ? c.unreadAlerts : id === 'messages' ? S.unreadMessages : 0;
    return `<div class="tile" onclick="location.hash='#/${id}'"><div class="ic ${tint}">${icon(ic)}</div>${label}${dot ? `<span class="dot">${dot}</span>` : ''}</div>`;
  }).join('')}</div>
  ${sectionCard('Latest updates', latest.length ? latest.map((n) => `<div class="list-item">${catIcon(n.category)}<div class="grow"><div class="t1">${esc(n.title)}</div><div class="t2">${esc(n.body)}</div></div><span class="muted small">${ago(n.created_at)}</span></div>`).join('') : '<div class="empty">No updates yet.</div>', `<a class="btn sm" href="#/alerts">View all</a>`)}
  <div class="grid g-2" style="gap:12px">
    <div class="card stat"><div class="ic tint-green">${icon('calendar')}</div><div><div class="l">Attendance</div><div class="v">${d.attendance_stats.overall || 0}%</div><div class="muted small">overall</div></div></div>
    <div class="card stat"><div class="ic tint-red">${icon('money')}</div><div><div class="l">Fees due</div><div class="v" style="font-size:19px">${money(d.fees.filter((f) => f.status === 'unpaid').reduce((a, f) => a + f.amount, 0))}</div><div class="muted small">${c.dueFees} pending</div></div></div>
  </div>`;
};

PV.attendance = (el, d) => {
  const s = d.attendance_stats;
  el.innerHTML = `<div class="grid g-2" style="gap:12px;grid-template-columns:repeat(3,1fr)">
    <div class="card stat" style="flex-direction:column;gap:2px"><div class="l">Present</div><div class="v" style="color:var(--success)">${s.present}</div><div class="muted small">this month</div></div>
    <div class="card stat" style="flex-direction:column;gap:2px"><div class="l">Absent</div><div class="v" style="color:var(--danger)">${s.absent}</div><div class="muted small">this month</div></div>
    <div class="card stat" style="flex-direction:column;gap:2px"><div class="l">Late</div><div class="v" style="color:var(--orange)">${s.late}</div><div class="muted small">this month</div></div>
  </div>
  ${sectionCard('Recent days', d.attendance.length ? d.attendance.map((a) => `<div class="list-item"><div class="grow"><div class="t1">${fmtDate(a.date)}</div><div class="t2">${new Date(a.date + 'T00:00').toLocaleDateString('en-GB', { weekday: 'long' })}</div></div>${badge(a.status)}</div>`).join('') : '<div class="empty">No attendance recorded yet.</div>', `<span class="badge blue">${s.overall || 0}% overall</span>`)}`;
};

PV.fees = (el, d) => {
  const due = d.fees.filter((f) => f.status === 'unpaid');
  el.innerHTML = `${due.length ? `<div class="card card-b" style="border-left:4px solid var(--danger)"><div class="muted small">Total due</div><div style="font-size:26px;font-weight:800">${money(due.reduce((a, f) => a + f.amount, 0))}</div><div class="muted small">Please pay at the school office or contact ${esc(S.school.phone || 'the school')}. Ignore reminders if already paid.</div></div>` : `<div class="card card-b">${badge('paid', 'All fees are paid')} Thank you!</div>`}
  ${sectionCard('Fee history', d.fees.length ? d.fees.map((f) => `<div class="list-item">${catIcon('fees')}<div class="grow"><div class="t1">${esc(f.title)}</div><div class="t2">${f.status === 'paid' ? 'Paid ' + fmtDate(f.paid_at) : 'Due ' + fmtDate(f.due_date)}</div></div><div style="text-align:right"><b>${money(f.amount)}</b><div>${f.overdue ? badge('overdue') : badge(f.status)}</div></div></div>`).join('') : '<div class="empty">No fees issued.</div>')}`;
};

PV.homework = (el, d) => {
  const t = todayStr();
  el.innerHTML = sectionCard('Homework diary', d.homework.length ? d.homework.map((h) => `<div class="list-item">${catIcon('homework')}<div class="grow"><div class="t1">${esc(h.subject)} · ${esc(h.title)}</div>${h.description ? `<div class="small">${esc(h.description)}</div>` : ''}<div class="t2">Due ${fmtDate(h.due_date)}${h.teacher_name ? ' · ' + esc(h.teacher_name) : ''}</div></div>${h.due_date >= t ? badge('late', 'Pending') : badge('', 'Past')}</div>`).join('') : '<div class="empty">No homework yet.</div>');
};

PV.exams = (el, d) => {
  el.innerHTML = sectionCard('Upcoming exams', d.exams.length ? d.exams.map((e) => `<div class="list-item">${catIcon('exams')}<div class="grow"><div class="t1">${esc(e.subject)} – ${esc(e.title)}</div><div class="t2">${fmtDate(e.exam_date)}${e.exam_time ? ' · ' + esc(e.exam_time) : ''} · Max ${e.max_marks}</div></div></div>`).join('') : '<div class="empty">No upcoming exams.</div>') +
    `<a class="btn block" href="#/results">${icon('chart')} View results</a>`;
};

PV.results = (el, d) => {
  el.innerHTML = sectionCard('Exam results', d.results.length ? d.results.map((r) => { const pct = Math.round((r.marks * 100) / r.max_marks); return `<div class="list-item">${catIcon('results')}<div class="grow"><div class="t1">${esc(r.subject)} – ${esc(r.title)}</div><div class="t2">${fmtDate(r.exam_date)} · Class average ${r.class_average ?? '—'}</div>${r.remarks ? `<div class="small">“${esc(r.remarks)}”</div>` : ''}<div class="hbar" style="grid-template-columns:1fr 60px;margin:6px 0 0"><div class="track"><div class="fill" style="width:${pct}%;background:${pct >= 75 ? 'var(--success)' : pct >= 50 ? 'var(--orange)' : 'var(--danger)'}"></div></div><b class="num">${r.marks}/${r.max_marks}</b></div></div><span class="badge blue" style="font-size:15px">${esc(r.grade || '')}</span></div>`; }).join('') : '<div class="empty">No published results yet.</div>');
};

PV.timetable = (el, d) => {
  const cfg = d.timetable_config;
  const todayRows = d.timetable.filter((t) => t.day === d.today_day);
  el.innerHTML = sectionCard('Today', d.today_day ? (todayRows.length ? todayRows.map((t) => `<div class="list-item"><div class="ic tint-blue"><b>P${t.period}</b></div><div class="grow"><div class="t1">${esc(t.subject_name)}</div><div class="t2">${esc(cfg.times[t.period - 1] || '')} · ${t.relief_teacher ? `${esc(t.relief_teacher)} <span class="badge amber">Relief teacher</span>` : esc(t.teacher_name || '')}</div></div></div>`).join('') : '<div class="empty">No timetable yet.</div>') : '<div class="empty">No school today.</div>') +
    `<div class="card"><div class="card-h"><h3>Week</h3></div>${d.timetable.length ? ttGrid(cfg, d.timetable.map((t) => ({ ...t, class_name: t.teacher_name })), { today: d.today_day }) : '<div class="empty">No timetable yet.</div>'}</div>` +
    (d.loans.length ? sectionCard('Library books', d.loans.map((l) => `<div class="list-item">${catIcon('library')}<div class="grow"><div class="t1">${esc(l.title)}</div><div class="t2">Return by ${fmtDate(l.due_date)}</div></div>${l.overdue ? badge('overdue') : ''}</div>`).join('')) : '');
};

PV.transport = (el, d) => {
  const s = d.student;
  el.innerHTML = s.route_name ? `<div class="card card-b"><div class="row">${catIcon('transport')}<div><b>${esc(s.route_name)}</b><div class="muted small">Bus ${esc(s.bus_no || '—')}</div></div></div>
    <div class="mt small">Driver: <b>${esc(s.driver_name || '—')}</b>${s.driver_phone ? ` · <a href="tel:${esc(s.driver_phone)}">${esc(s.driver_phone)}</a>` : ''}</div></div>
    ${sectionCard('Bus updates', d.bus_updates.length ? d.bus_updates.map((b) => `<div class="list-item">${catIcon('transport')}<div class="grow"><div class="t2" style="color:var(--text)">${esc(b.message)}</div></div><span class="muted small">${ago(b.created_at)}</span></div>`).join('') : '<div class="empty">No updates.</div>')}`
    : '<div class="card empty">Your child is not assigned to a school bus route.</div>';
};

PV.alerts = async (el) => {
  const list = await GET('/api/notifications');
  el.innerHTML = sectionCard('Notifications', list.length ? list.map((n) => `<div class="list-item">${catIcon(n.category)}<div class="grow"><div class="t1">${esc(n.title)} ${n.is_read ? '' : '<span class="badge blue">New</span>'}</div><div class="t2">${esc(n.body)}</div></div><span class="muted small">${ago(n.created_at)}</span></div>`).join('') : '<div class="empty">No notifications yet.</div>');
  if (list.some((n) => !n.is_read)) { POST('/api/notifications/read-all'); P.data && P.data.notifications.forEach((n) => (n.is_read = 1)); }
};

PV.messages = async (el) => {
  await messenger(el, location.hash.split('/')[2]);
  const r = await GET('/api/me'); S.unreadMessages = r.unread_messages;
};

PV.ai = (el) => {
  if (!S.aiEnabled) { el.innerHTML = '<div class="card empty">AI assistant is not enabled by the school.</div>'; return; }
  aiChat(el, ['How is my child doing this month?', 'Are there any fees due?', 'What homework is pending?', 'මගේ දරුවාගේ පැමිණීම කොහොමද?', 'என் குழந்தையின் தேர்வு முடிவுகள் என்ன?']);
};

PV.profile = (el) => {
  el.innerHTML = `<div class="card card-b row"><div class="avatar" style="width:54px;height:54px;font-size:19px">${esc(initials(S.me.name))}</div><div><b style="font-size:17px">${esc(S.me.name)}</b><div class="muted small">${esc(S.me.email)} · ${cap(S.me.role)}</div></div></div>
  <div class="card card-b"><h3 style="margin-bottom:12px">Language</h3>${langPicker('wide')}<p class="muted small" style="margin:10px 0 0">The app and your notifications will be in this language.</p></div>
  <form class="card card-b" id="pf"><h3 style="margin-bottom:12px">Contact details</h3>
    ${field({ name: 'phone', label: 'Mobile', value: S.me.phone })}
    ${field({ name: 'language', label: 'Preferred language', type: 'select', options: ['English', 'Sinhala', 'Tamil'], value: S.me.language })}
    <button class="btn primary">Save</button></form>
  <form class="card card-b" id="pw"><h3 style="margin-bottom:12px">Change password</h3>
    ${field({ name: 'current', label: 'Current password', type: 'password', autocomplete: 'current-password' })}${field({ name: 'new', label: 'New password (min 8 characters)', type: 'password', autocomplete: 'new-password' })}
    <button class="btn">Update password</button></form>
  <div class="card card-b small"><b>${esc(S.school.name)}</b><div class="muted">${esc(S.school.address || '')}<br>${esc(S.school.phone || '')} · ${esc(S.school.email || '')}</div></div>
  <div class="row"><button class="btn" onclick="toggleTheme()">${icon('moon')} Dark mode</button><button class="btn danger right" onclick="logout()">${icon('logout')} Sign out</button></div>`;
  $('#pf', el).addEventListener('submit', async (e) => { e.preventDefault(); const v = readForm(e.target); try { await PUT('/api/me', v); Object.assign(S.me, v); toast(t('Saved'), 'success'); if (LANG_CODES[v.language] !== LANG) setLang(LANG_CODES[v.language], false); } catch (err) { toast(err.message, 'error'); } });
  $('#pw', el).addEventListener('submit', async (e) => { e.preventDefault(); try { await POST('/api/me/password', readForm(e.target)); e.target.reset(); toast('Password updated', 'success'); } catch (err) { toast(err.message, 'error'); } });
};
