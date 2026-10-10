/* Staff console: timetable, teacher attendance, leave, substitutions, admissions, library */
'use strict';

const LEAVE_TYPES = [['casual', 'Casual leave'], ['medical', 'Medical leave'], ['duty', 'Duty leave'], ['half_day', 'Half day'], ['maternity', 'Maternity leave'], ['other', 'Other']];
const dateRange = (a, b) => fmtDate(a) + (b && b !== a ? ' – ' + fmtDate(b) : '');

function applyLeaveModal(done) {
  modal({
    title: 'Apply for leave', submit: 'Send request',
    body: formFields([
      { name: 'leave_type', label: 'Leave type', type: 'select', options: LEAVE_TYPES, full: true },
      { name: 'from_date', label: 'From', type: 'date', required: true, value: todayStr() },
      { name: 'to_date', label: 'To', type: 'date', required: true, value: todayStr() },
      { name: 'reason', label: 'Reason', type: 'textarea', rows: 3, full: true },
    ]) + '<p class="muted small">Your request goes to the principal, vice principal and your head of section. Once approved, free teachers are automatically assigned to cover your periods.</p>',
    onSubmit: async (v) => { await POST('/api/leave', v); toast('Leave request sent', 'success'); done && done(); },
  });
}

function decideLeave(id, status, done) {
  modal({
    title: status === 'approved' ? 'Approve leave' : 'Reject leave', submit: status === 'approved' ? 'Approve' : 'Reject',
    body: field({ name: 'note', label: 'Note to the staff member (optional)', type: 'textarea', rows: 2 }),
    onSubmit: async (v) => {
      const r = await POST(`/api/leave/${id}/decision`, { status, note: v.note });
      let msg = 'Leave ' + status;
      if (r.substitutions_assigned !== undefined) msg += ` · ${r.substitutions_assigned} relief period(s) assigned${r.substitutions_unfilled ? `, ${r.substitutions_unfilled} need cover` : ''}`;
      toast(msg, r.substitutions_unfilled ? '' : 'success');
      S.pendingLeave = Math.max(0, (S.pendingLeave || 1) - 1);
      done && done();
    },
  });
}

/* ---------- Timetable grid ---------- */
function ttGrid(cfg, entries, { cell, today } = {}) {
  const at = {};
  entries.forEach((e) => { (at[`${e.day}-${e.period}`] ||= []).push(e); });
  let html = `<div class="table-wrap"><table class="t tt"><thead><tr><th>Period</th>${cfg.day_names.map((d, i) => `<th class="${today === i + 1 ? 'tt-today' : ''}">${d.slice(0, 3)}</th>`).join('')}</tr></thead><tbody>`;
  for (let p = 1; p <= cfg.periods; p++) {
    html += `<tr><td class="tt-p"><b>P${p}</b><div class="muted small">${esc(cfg.times[p - 1])}</div></td>`;
    for (let d = 1; d <= cfg.days; d++) {
      const list = at[`${d}-${p}`] || [];
      html += `<td class="tt-c ${today === d ? 'tt-today' : ''}" data-day="${d}" data-period="${p}">${cell ? cell(list, d, p) : list.map((e) => `<div class="tt-s" style="--h:${hue(e.subject_name)}"><b>${esc(e.subject_name)}</b><span>${esc(e.class_name || e.teacher_name || '')}</span></div>`).join('')}</td>`;
    }
    html += '</tr>';
    if (p === cfg.break_after && p < cfg.periods) html += `<tr class="tt-break"><td colspan="${cfg.days + 1}">${t('Interval')} · ${cfg.break_minutes} ${t('min')}</td></tr>`;
  }
  return html + '</tbody></table></div>';
}
const hue = (s) => { let h = 0; for (const c of String(s)) h = (h * 31 + c.charCodeAt(0)) % 360; return h; };

PAGES.timetable = async (el) => {
  const edit = can('timetable.edit');
  const parts = location.hash.split('/');
  const tabs = [['class', 'Class timetable'], ['teacher', 'Teacher timetable'], ...(edit ? [['alloc', 'Subjects & teachers']] : []), ...(can('setup') ? [['subjects', 'Subject list']] : [])];
  let tab = tabs.some((t) => t[0] === parts[2]) ? parts[2] : (S.teaching && !edit ? 'teacher' : 'class');
  el.innerHTML = `<div class="row" style="margin-bottom:16px"><div class="tabs" id="tt">${tabs.map(([v, l]) => `<button data-v="${v}" class="${v === tab ? 'active' : ''}">${l}</button>`).join('')}</div>
    <span class="spacer"></span>${edit ? `<button class="btn ai" id="gen">${icon('sparkle')} Generate timetable</button>` : ''}</div><div id="tbody">${loading()}</div>`;
  const box = $('#tbody', el);
  const show = () => ({ class: classView, teacher: teacherView, alloc: allocView, subjects: subjectsView })[tab]().catch((e) => { box.innerHTML = `<div class="card empty">${esc(e.message)}</div>`; });
  $$('#tt button', el).forEach((b) => b.onclick = () => { tab = b.dataset.v; history.replaceState(null, '', '#/timetable/' + tab); $$('#tt button', el).forEach((x) => x.classList.toggle('active', x === b)); show(); });
  const gen = $('#gen', el);
  gen && (gen.onclick = async () => {
    if (!(await confirmBox('Generate a new timetable for all classes? Existing periods will be replaced and relief duties recalculated.', 'Generate'))) return;
    gen.disabled = true;
    try {
      const r = await POST('/api/timetable/generate', {});
      modal({ title: 'Timetable generated', footer: false, body: `<p><b>${r.placed}</b> periods scheduled with no teacher clashes.</p>${r.unplaced.length ? `<p style="color:var(--danger)"><b>Could not place:</b></p><ul>${r.unplaced.map((u) => `<li>${esc(u)}</li>`).join('')}</ul><p class="muted small">Reduce periods per week, add another teacher for the subject, or increase periods per day in Settings.</p>` : '<p class="muted">Every allocated period was placed.</p>'}${r.warnings.map((w) => `<p class="small" style="color:var(--warning)">${esc(w)}</p>`).join('')}` });
      show();
    } catch (e) { toast(e.message, 'error'); } finally { gen.disabled = false; }
  });

  async function classView() {
    await loadClasses(false, edit || !S.teaching ? '' : 'academic');
    if (!S.classes.length) { box.innerHTML = '<div class="card empty">No classes yet.</div>'; return; }
    const pick = S.classes.some((c) => c.id == parts[3]) ? parts[3] : S.classes[0].id;
    box.innerHTML = `<div class="card"><div class="card-h"><select class="input" id="tc" style="width:auto">${classOptions().map(([v, l]) => `<option value="${v}" ${v == pick ? 'selected' : ''}>${esc(l)}</option>`).join('')}</select>
      ${edit ? '<span class="muted small">Click a period to change it</span>' : ''}<span class="spacer"></span><button class="btn sm" onclick="window.print()">Print</button></div><div id="tg">${loading()}</div></div>`;
    const load = async () => {
      const cid = $('#tc', box).value;
      const [d, alloc] = await Promise.all([GET('/api/timetable?class_id=' + cid), edit ? GET('/api/class-subjects?class_id=' + cid) : []]);
      const today = d.config.day_names.indexOf(new Date().toLocaleDateString('en-GB', { weekday: 'long' })) + 1;
      $('#tg', box).innerHTML = d.entries.length || edit ? ttGrid(d.config, d.entries.map((e) => ({ ...e, class_name: e.teacher_name || '—' })), { today }) : '<div class="empty">No timetable yet.</div>';
      if (edit) $$('.tt-c', box).forEach((td) => td.onclick = () => modal({
        title: `${d.config.day_names[td.dataset.day - 1]} · Period ${td.dataset.period}`, submit: 'Save',
        body: field({ name: 'subject_id', label: 'Subject', type: 'select', options: [[0, '— Free period —'], ...alloc.map((a) => [a.subject_id, `${a.subject_name} (${a.teacher_name || 'no teacher'})`])], value: (d.entries.find((e) => e.day == td.dataset.day && e.period == td.dataset.period) || {}).subject_id }),
        onSubmit: async (v) => { await PUT('/api/timetable/slot', { class_id: Number(cid), day: Number(td.dataset.day), period: Number(td.dataset.period), subject_id: Number(v.subject_id) }); load(); },
      }));
    };
    $('#tc', box).onchange = load;
    await load();
  }

  async function teacherView() {
    const staff = await GET('/api/staff?teaching=1');
    const me = staff.some((t) => t.id === S.me.id) ? S.me.id : (staff[0] || {}).id;
    box.innerHTML = `<div class="card"><div class="card-h"><select class="input" id="tt2" style="width:auto">${staff.map((t) => `<option value="${t.id}" ${t.id === me ? 'selected' : ''}>${esc(t.name)} · ${t.periods} periods</option>`).join('')}</select><span class="spacer"></span><button class="btn sm" onclick="window.print()">Print</button></div><div id="tg">${loading()}</div></div>`;
    const load = async () => {
      const d = await GET('/api/timetable?teacher_id=' + $('#tt2', box).value);
      const today = d.config.day_names.indexOf(new Date().toLocaleDateString('en-GB', { weekday: 'long' })) + 1;
      $('#tg', box).innerHTML = d.entries.length ? ttGrid(d.config, d.entries, { today }) : '<div class="empty">No periods assigned to this teacher.</div>';
    };
    $('#tt2', box).onchange = load;
    if (staff.length) await load(); else $('#tg', box).innerHTML = '<div class="empty">No teaching staff yet.</div>';
  }

  async function allocView() {
    const [classes, subjects, teachers, tt] = await Promise.all([loadClasses(true), GET('/api/subjects'), GET('/api/staff?teaching=1'), GET('/api/timetable')]);
    const slots = tt.config.days * tt.config.periods;
    const pick = classes.some((c) => c.id == parts[3]) ? parts[3] : (classes[0] || {}).id;
    box.innerHTML = classes.length ? `<div class="grid g-main"><div class="card"><div class="card-h"><select class="input" id="ac" style="width:auto">${classes.map((c) => `<option value="${c.id}" ${c.id == pick ? 'selected' : ''}>${esc(c.label)}</option>`).join('')}</select>
      <span id="aSum" class="muted small"></span><span class="spacer"></span><button class="btn primary" id="aAdd">${icon('plus')} Add subject</button></div><div id="al">${loading()}</div></div>
      <div class="card"><div class="card-h"><h3>Teacher load</h3><span class="muted small">periods per week</span></div><div id="load" class="card-b">${loading()}</div></div></div>` : '<div class="card empty">Create classes first (Sections & Classes).</div>';
    if (!classes.length) return;
    const load = async () => {
      const cid = $('#ac', box).value;
      const [rows, all] = await Promise.all([GET('/api/class-subjects?class_id=' + cid), GET('/api/class-subjects')]);
      const total = rows.reduce((a, r) => a + r.periods_per_week, 0);
      $('#aSum', box).innerHTML = `<b style="color:${total > slots ? 'var(--danger)' : 'inherit'}">${total}</b> of ${slots} periods a week allocated`;
      $('#al', box).innerHTML = table([
        { label: 'Subject', render: (r) => `<b>${esc(r.subject_name)}</b>` }, { label: 'Teacher', render: (r) => r.teacher_name ? esc(r.teacher_name) : '<span class="badge red">No teacher</span>' },
        { label: 'Periods / week', key: 'periods_per_week', num: true },
        { label: '', render: (r) => `<div class="row" style="justify-content:flex-end"><button class="btn sm" data-ae="${r.id}">${icon('edit')}</button><button class="btn sm danger" data-ad="${r.id}">${icon('trash')}</button></div>` },
      ], rows, 'No subjects allocated to this class yet.');
      const loadBy = {};
      all.forEach((r) => { if (r.teacher_name) loadBy[r.teacher_name] = (loadBy[r.teacher_name] || 0) + r.periods_per_week; });
      const max = Math.max(1, ...Object.values(loadBy));
      $('#load', box).innerHTML = Object.entries(loadBy).sort((a, b) => b[1] - a[1]).map(([n, v]) => `<div class="hbar" style="grid-template-columns:130px 1fr 40px"><span class="small">${esc(n)}</span><div class="track"><div class="fill" style="width:${(v / max) * 100}%;background:${v > slots * 0.8 ? 'var(--danger)' : 'var(--brand)'}"></div></div><b class="num small">${v}</b></div>`).join('') || '<div class="muted small">No teachers allocated yet.</div>';
      $$('[data-ae]', box).forEach((b) => b.onclick = () => form(rows.find((r) => r.id == b.dataset.ae)));
      $$('[data-ad]', box).forEach((b) => b.onclick = async () => { if (await confirmBox('Remove this subject from the class? Its timetable periods are removed too.', 'Remove')) DEL('/api/class-subjects/' + b.dataset.ad).then(load).catch((e) => toast(e.message, 'error')); });
    };
    const form = (r = {}) => modal({
      title: r.id ? 'Edit subject allocation' : 'Add subject to class',
      body: subjects.length ? formFields([
        { name: 'subject_id', label: 'Subject', type: 'select', full: true, options: subjects.map((s) => [s.id, s.name]) },
        { name: 'teacher_id', label: 'Teacher', type: 'select', full: true, options: [[0, '— Not assigned —'], ...teachers.map((t) => [t.id, `${t.name} (${t.role_label}) · ${t.periods} periods`])] },
        { name: 'periods_per_week', label: 'Periods per week', type: 'number', min: 1, value: r.periods_per_week || 3 },
      ], r) : '<p>Add subjects in the "Subject list" tab first.</p>',
      onSubmit: async (v) => { await POST('/api/class-subjects', { class_id: Number($('#ac', box).value), subject_id: Number(v.subject_id), teacher_id: Number(v.teacher_id), periods_per_week: Number(v.periods_per_week) }); toast('Saved — generate the timetable to apply changes', 'success'); load(); },
    });
    $('#ac', box).onchange = load;
    $('#aAdd', box).onclick = () => form();
    await load();
  }

  async function subjectsView() {
    const rows = await GET('/api/subjects');
    box.innerHTML = `<div class="card"><div class="card-h"><h3>Subjects</h3><span class="spacer"></span><button class="btn primary" id="sAdd">${icon('plus')} Add subject</button></div>${table([
      { label: 'Subject', render: (r) => `<b>${esc(r.name)}</b>` }, { label: 'Code', key: 'code' }, { label: 'Classes', key: 'class_count', num: true },
      { label: '', render: (r) => `<div class="row" style="justify-content:flex-end"><button class="btn sm" data-se="${r.id}">${icon('edit')}</button><button class="btn sm danger" data-sd="${r.id}">${icon('trash')}</button></div>` },
    ], rows, 'No subjects yet.')}</div>`;
    const form = (s = {}) => modal({ title: s.id ? 'Edit subject' : 'Add subject', body: formFields([{ name: 'name', label: 'Subject name', required: true }, { name: 'code', label: 'Code', placeholder: 'MAT' }], s),
      onSubmit: async (v) => { s.id ? await PUT('/api/subjects/' + s.id, v) : await POST('/api/subjects', v); subjectsView(); } });
    $('#sAdd', box).onclick = () => form();
    $$('[data-se]', box).forEach((b) => b.onclick = () => form(rows.find((r) => r.id == b.dataset.se)));
    $$('[data-sd]', box).forEach((b) => b.onclick = async () => { if (await confirmBox('Delete this subject from every class and timetable?', 'Delete')) DEL('/api/subjects/' + b.dataset.sd).then(subjectsView).catch((e) => toast(e.message, 'error')); });
  }
  await show();
};

/* ---------- My Day ---------- */
PAGES.workspace = async (el) => {
  const [w, leave] = await Promise.all([GET('/api/workspace'), GET('/api/leave?mine=1')]);
  const a = w.attendance;
  const status = w.on_leave ? badge('leave', 'On leave today') : a.status ? badge(a.status === 'late' ? 'late' : a.status, cap(a.status)) : badge('', 'Not checked in');
  el.innerHTML = `<div class="stack">
    <div class="grid g-2">
      <div class="card"><div class="card-h"><h3>${greeting()}, ${esc(S.me.name.split(' ')[0])}</h3><span class="spacer"></span>${status}</div><div class="card-b row">
        <div><div class="muted small">Checked in</div><b style="font-size:22px">${esc(a.check_in || '—')}</b></div>
        <div><div class="muted small">Checked out</div><b style="font-size:22px">${esc(a.check_out || '—')}</b></div>
        <span class="spacer"></span>
        ${w.on_leave ? '' : !a.check_in ? `<button class="btn primary" id="cin">${icon('check')} Check in</button>` : !a.check_out ? `<button class="btn" id="cout">${icon('logout')} Check out</button>` : badge('present', 'Done for today')}
      </div><div class="card-b muted small" style="padding-top:0">Arrivals after ${esc(w.late_after)} are recorded as late.</div></div>
      <div class="card"><div class="card-h"><h3>My leave</h3><span class="spacer"></span><button class="btn primary" id="applyL">${icon('plus')} Apply for leave</button></div>
        ${leave.slice(0, 4).map((l) => `<div class="list-item">${catIcon('leave')}<div class="grow"><div class="t1">${esc(l.type_label)}</div><div class="t2">${dateRange(l.from_date, l.to_date)}${l.review_note ? ' · “' + esc(l.review_note) + '”' : ''}</div></div>${badge(l.status)}</div>`).join('') || '<div class="empty">No leave requests.</div>'}
        ${leave.length > 4 ? '<div class="card-b"><a class="btn sm" href="#/leave">All my leave</a></div>' : ''}
      </div>
    </div>
    <div class="grid g-2">
      <div class="card"><div class="card-h"><h3>My periods today</h3><span class="muted small">${esc(w.day_name)}</span></div>
        ${!w.school_day ? '<div class="empty">No school today.</div>' : w.lessons.length ? w.lessons.map((l) => `<div class="list-item"><div class="ic tint-blue"><b>P${l.period}</b></div><div class="grow"><div class="t1">${esc(l.subject_name)} · ${esc(l.class_name)}</div><div class="t2">${esc(l.time)}${l.covered_by ? ' · covered by ' + esc(l.covered_by) : ''}</div></div>${l.covered_by ? badge('leave', 'Covered') : ''}</div>`).join('') : '<div class="empty">No periods today.</div>'}
      </div>
      <div class="card"><div class="card-h"><h3>Relief duties</h3><span class="muted small">periods you cover for absent teachers</span></div>
        ${w.relief.map((r) => `<div class="list-item"><div class="ic tint-violet"><b>P${r.period}</b></div><div class="grow"><div class="t1">${esc(r.class_name)} · ${esc(r.subject_name)}</div><div class="t2">${esc(r.time)} · for ${esc(r.absent_name)}</div></div>${badge('assigned', 'Today')}</div>`).join('')}
        ${w.upcoming_relief.map((r) => `<div class="list-item"><div class="ic tint-violet"><b>P${r.period}</b></div><div class="grow"><div class="t1">${esc(r.class_name)} · ${esc(r.subject_name)}</div><div class="t2">${fmtDate(r.date)}</div></div></div>`).join('')}
        ${!w.relief.length && !w.upcoming_relief.length ? '<div class="empty">No relief duties.</div>' : ''}
      </div>
    </div>
    ${S.teaching ? `<div class="card"><div class="card-h"><h3>My weekly timetable</h3></div><div id="myTT">${loading()}</div></div>` : ''}
  </div>`;
  const ci = $('#cin', el), co = $('#cout', el);
  ci && (ci.onclick = async () => { try { const r = await POST('/api/staff-attendance/check-in', { action: 'in' }); toast(`Checked in at ${r.time}${r.status === 'late' ? ' (late)' : ''}`, 'success'); PAGES.workspace(el); } catch (e) { toast(e.message, 'error'); } });
  co && (co.onclick = async () => { try { const r = await POST('/api/staff-attendance/check-in', { action: 'out' }); toast('Checked out at ' + r.time, 'success'); PAGES.workspace(el); } catch (e) { toast(e.message, 'error'); } });
  $('#applyL', el).onclick = () => applyLeaveModal(() => PAGES.workspace(el));
  if (S.teaching) {
    const d = await GET('/api/timetable?teacher_id=' + S.me.id);
    const today = d.config.day_names.indexOf(new Date().toLocaleDateString('en-GB', { weekday: 'long' })) + 1;
    $('#myTT', el).innerHTML = d.entries.length ? ttGrid(d.config, d.entries, { today }) : '<div class="empty">No periods assigned yet.</div>';
  }
};

/* ---------- Teacher attendance ---------- */
PAGES.staffatt = async (el) => {
  const mark = can('staff.mark');
  el.innerHTML = `<div class="card"><div class="card-h"><input class="input" type="date" id="sd" value="${todayStr()}" max="${todayStr()}" style="width:auto">
    <div class="tabs" id="sf"><button class="active" data-v="teaching">Teachers</button><button data-v="all">All staff</button></div>
    <span class="spacer"></span>${mark ? `<button class="btn sm" id="allP">${icon('check')} Mark unmarked as present</button>` : ''}</div>
    <div id="ss" class="card-b row" style="border-bottom:1px solid var(--border)"></div><div id="sl">${loading()}</div>
    ${mark ? `<div class="card-b row" style="border-top:1px solid var(--border)"><span class="muted small">Marking a teacher absent automatically assigns free teachers to cover their periods.</span><span class="spacer"></span><button class="btn primary" id="save">${icon('check')} Save</button></div>` : ''}</div>`;
  let who = 'teaching', data = null;
  const state = {};
  const render = () => {
    const rows = data.staff.filter((s) => who === 'all' || s.teaching);
    const c = (st) => rows.filter((r) => (r.leave_type ? 'leave' : state[r.user_id]) === st).length;
    $('#ss', el).innerHTML = `<span class="badge green">Present ${c('present')}</span><span class="badge amber">Late ${c('late')}</span><span class="badge blue">On leave ${c('leave')}</span><span class="badge red">Absent ${c('absent')}</span><span class="badge">Not marked ${c('')}</span>`;
    $('#sl', el).innerHTML = table([
      { label: 'Staff member', render: (r) => `<div class="row" style="flex-wrap:nowrap"><div class="avatar">${esc(initials(r.name))}</div><div><b>${esc(r.name)}</b><div class="muted small">${esc(r.role_label)}</div></div></div>` },
      { label: 'Check in', render: (r) => esc(r.check_in || '—') }, { label: 'Check out', render: (r) => esc(r.check_out || '—') },
      { label: 'Status', render: (r) => r.leave_type ? badge('leave', 'On approved leave · ' + cap(r.leave_type)) : mark ? `<div class="seg" data-id="${r.user_id}">${['present', 'late', 'absent'].map((s) => `<button type="button" data-s="${s}" class="${state[r.user_id] === s ? 'on-' + (s === 'late' ? 'late' : s) : ''}">${cap(s)}</button>`).join('')}</div>` : (state[r.user_id] ? badge(state[r.user_id]) : badge('', 'Not marked')) },
    ], rows, 'No staff.');
  };
  const load = async () => {
    data = await GET('/api/staff-attendance?date=' + $('#sd', el).value);
    Object.keys(state).forEach((k) => delete state[k]);
    data.staff.forEach((s) => { state[s.user_id] = s.status === 'leave' ? '' : s.status; });
    render();
  };
  el.addEventListener('click', (e) => { const b = e.target.closest('.seg button'); if (b) { state[b.parentElement.dataset.id] = b.dataset.s; render(); } });
  $$('#sf button', el).forEach((b) => b.onclick = () => { who = b.dataset.v; $$('#sf button', el).forEach((x) => x.classList.toggle('active', x === b)); render(); });
  $('#sd', el).onchange = load;
  if (mark) {
    $('#allP', el).onclick = () => { data.staff.forEach((s) => { if (!state[s.user_id] && !s.leave_type && (who === 'all' || s.teaching)) state[s.user_id] = 'present'; }); render(); };
    $('#save', el).onclick = async () => {
      const records = data.staff.filter((s) => state[s.user_id] && !s.leave_type && state[s.user_id] !== s.status).map((s) => ({ user_id: s.user_id, status: state[s.user_id] }));
      if (!records.length) return toast('No changes to save');
      try {
        const r = await POST('/api/staff-attendance', { date: $('#sd', el).value, records });
        toast(`Saved${r.substitutions_assigned !== undefined ? ` · ${r.substitutions_assigned} relief period(s) assigned${r.substitutions_unfilled ? `, ${r.substitutions_unfilled} uncovered` : ''}` : ''}`, 'success');
        load();
      } catch (e) { toast(e.message, 'error'); }
    };
  }
  await load();
};

/* ---------- Leave requests ---------- */
PAGES.leave = async (el) => {
  const approver = can('leave.approve');
  el.innerHTML = `<div class="card"><div class="card-h"><div class="tabs" id="lt">${[['pending', 'Pending'], ['approved', 'Approved'], ['rejected', 'Rejected'], ['', 'All']].map(([v, l], i) => `<button data-v="${v}" class="${i === 0 ? 'active' : ''}">${l}</button>`).join('')}</div>
    ${approver ? '<label class="check small"><input type="checkbox" id="mine"> Only mine</label>' : ''}<span class="spacer"></span><button class="btn primary" id="apply">${icon('plus')} Apply for leave</button></div><div id="ll">${loading()}</div></div>`;
  let status = 'pending';
  const load = async () => {
    const mine = !approver || $('#mine', el).checked;
    const rows = await GET(`/api/leave?status=${status}${mine ? '&mine=1' : ''}`);
    $('#ll', el).innerHTML = table([
      ...(approver && !mine ? [{ label: 'Staff member', render: (r) => `<b>${esc(r.name)}</b><div class="muted small">${esc(r.role_label)}</div>` }] : []),
      { label: 'Type', render: (r) => esc(r.type_label) }, { label: 'Dates', render: (r) => `${dateRange(r.from_date, r.to_date)} <span class="muted small">(${r.days} day${r.days > 1 ? 's' : ''})</span>` },
      { label: 'Reason', render: (r) => `<span class="small">${esc(r.reason)}</span>` },
      { label: 'Status', render: (r) => badge(r.status) + (r.reviewer_name ? `<div class="muted small">by ${esc(r.reviewer_name)}${r.review_note ? ': ' + esc(r.review_note) : ''}</div>` : '') },
      { label: '', render: (r) => `<div class="row" style="flex-wrap:nowrap;justify-content:flex-end">
        ${approver && r.user_id !== S.me.id && r.status === 'pending' ? `<button class="btn sm success" data-ok="${r.id}">${icon('check')} Approve</button><button class="btn sm danger" data-no="${r.id}">${icon('x')} Reject</button>` : ''}
        ${approver && r.user_id !== S.me.id && r.status === 'approved' && r.to_date >= todayStr() ? `<button class="btn sm danger" data-no="${r.id}">Revoke</button>` : ''}
        ${r.user_id === S.me.id && (r.status === 'pending' || (r.status === 'approved' && r.to_date >= todayStr())) ? `<button class="btn sm" data-cancel="${r.id}">Cancel</button>` : ''}</div>` },
    ], rows, 'No leave requests.');
    $$('[data-ok]', el).forEach((b) => b.onclick = () => decideLeave(b.dataset.ok, 'approved', load));
    $$('[data-no]', el).forEach((b) => b.onclick = () => decideLeave(b.dataset.no, 'rejected', load));
    $$('[data-cancel]', el).forEach((b) => b.onclick = async () => { if (await confirmBox('Cancel this leave request?', 'Cancel request')) POST(`/api/leave/${b.dataset.cancel}/cancel`).then(() => { toast('Cancelled'); load(); }).catch((e) => toast(e.message, 'error')); });
  };
  $$('#lt button', el).forEach((b) => b.onclick = () => { status = b.dataset.v; $$('#lt button', el).forEach((x) => x.classList.toggle('active', x === b)); load(); });
  const mineBox = $('#mine', el); mineBox && (mineBox.onchange = load);
  $('#apply', el).onclick = () => applyLeaveModal(load);
  await load();
};

/* ---------- Substitutions ---------- */
PAGES.subs = async (el) => {
  const manage = can('substitutions');
  el.innerHTML = `<div class="card"><div class="card-h"><input class="input" type="date" id="xd" value="${todayStr()}" style="width:auto"><span class="spacer"></span>
    ${manage ? `<button class="btn primary" id="auto">${icon('refresh')} Auto-assign relief teachers</button>` : ''}</div><div id="xa" class="card-b" style="border-bottom:1px solid var(--border)"></div><div id="xl">${loading()}</div></div>`;
  const load = async () => {
    const d = await GET('/api/substitutions?date=' + $('#xd', el).value);
    $('#xa', el).innerHTML = !d.school_day ? '<span class="muted">Not a school day.</span>' : d.away.length ? `<span class="muted small">Away:</span> ${d.away.map((a) => `<span class="badge blue">${esc(a.name)}</span>`).join(' ')}` : '<span class="muted">All teachers are available.</span>';
    $('#xl', el).innerHTML = table([
      { label: 'Period', render: (r) => `<b>P${r.period}</b><div class="muted small">${esc(r.time)}</div>` }, { label: 'Class', render: (r) => `<b>${esc(r.class_name)}</b>` },
      { label: 'Subject', render: (r) => esc(r.subject_name || '') }, { label: 'Absent teacher', render: (r) => esc(r.absent_name || '') },
      { label: 'Relief teacher', render: (r) => r.substitute_name ? `<b>${esc(r.substitute_name)}</b>` : badge('unfilled', 'No free teacher') },
      { label: '', render: (r) => manage ? `<button class="btn sm" data-ch="${r.id}" data-p="${r.period}">${icon('edit')} Change</button>` : '' },
    ], d.rows, d.school_day ? 'No periods need cover on this day.' : '');
    $$('[data-ch]', el).forEach((b) => b.onclick = async () => {
      const free = await GET(`/api/substitutions/free-teachers?date=${$('#xd', el).value}&period=${b.dataset.p}`);
      modal({ title: 'Choose relief teacher', submit: 'Assign',
        body: field({ name: 'sub', label: `Teachers free in period ${b.dataset.p}`, type: 'select', options: [[0, '— Leave uncovered —'], ...free.map((t) => [t.id, `${t.name} (${t.role_label})`])] }),
        onSubmit: async (v) => { await PUT('/api/substitutions/' + b.dataset.ch, { substitute_id: Number(v.sub) }); toast('Relief teacher updated and notified', 'success'); load(); } });
    });
  };
  $('#xd', el).onchange = load;
  const auto = $('#auto', el);
  auto && (auto.onclick = async () => { try { const r = await POST('/api/substitutions/auto', { date: $('#xd', el).value }); toast(`${r.assigned} period(s) covered${r.unfilled ? `, ${r.unfilled} without a free teacher` : ''}`, r.unfilled ? '' : 'success'); load(); } catch (e) { toast(e.message, 'error'); } });
  await load();
};

/* ---------- Admissions ---------- */
const ADM = [['enquiry', 'Enquiry'], ['test_scheduled', 'Test scheduled'], ['offered', 'Offered'], ['enrolled', 'Enrolled'], ['rejected', 'Rejected'], ['withdrawn', 'Withdrawn']];
PAGES.admissions = async (el) => {
  el.innerHTML = `<div class="card"><div class="card-h"><div class="tabs" id="at"><button class="active" data-v="">All</button>${ADM.map(([v, l]) => `<button data-v="${v}">${l} <span class="muted small" data-n="${v}"></span></button>`).join('')}</div>
    <input class="input" id="aq" placeholder="Search…" style="max-width:200px"><span class="spacer"></span><button class="btn primary" id="add">${icon('plus')} New enquiry</button></div><div id="al">${loading()}</div></div>`;
  let status = '';
  const load = async () => {
    const d = await GET(`/api/admissions?status=${status}&q=${encodeURIComponent($('#aq', el).value)}`);
    $$('[data-n]', el).forEach((s) => { const c = d.counts.find((x) => x.status === s.dataset.n); s.textContent = c ? c.n : ''; });
    $('#al', el).innerHTML = table([
      { label: 'Child', render: (r) => `<b>${esc(r.child_name)}</b><div class="muted small">${esc(r.grade_applying)}${r.previous_school ? ' · from ' + esc(r.previous_school) : ''}</div>` },
      { label: 'Parent', render: (r) => `${esc(r.parent_name)}<div class="muted small">${esc(r.phone)} ${esc(r.email)}</div>` },
      { label: 'Status', render: (r) => badge(r.status, (ADM.find((x) => x[0] === r.status) || [, r.status])[1]) + (r.admission_no ? `<div class="muted small">${esc(r.admission_no)}</div>` : '') },
      { label: 'Follow up', render: (r) => r.follow_up ? `<span style="color:${r.follow_up < todayStr() && r.status !== 'enrolled' ? 'var(--danger)' : 'inherit'}">${fmtDate(r.follow_up)}</span>` : '<span class="muted">—</span>' },
      { label: 'Notes', render: (r) => `<span class="small">${esc(r.notes)}</span>` },
      { label: '', render: (r) => r.status === 'enrolled' ? '' : `<div class="row" style="flex-wrap:nowrap;justify-content:flex-end"><button class="btn sm" data-e="${r.id}">${icon('edit')}</button><button class="btn sm success" data-en="${r.id}">Enrol</button></div>` },
    ], d.rows, 'No applications.');
    $$('[data-e]', el).forEach((b) => b.onclick = () => form(d.rows.find((r) => r.id == b.dataset.e)));
    $$('[data-en]', el).forEach((b) => b.onclick = () => enrol(d.rows.find((r) => r.id == b.dataset.en)));
  };
  const form = (r = {}) => modal({
    title: r.id ? 'Edit application' : 'New enquiry', wide: true,
    body: formFields([
      { name: 'child_name', label: 'Child name', required: true }, { name: 'grade_applying', label: 'Grade applying for', placeholder: 'Grade 1' },
      { name: 'dob', label: 'Date of birth', type: 'date' }, { name: 'gender', label: 'Gender', type: 'select', options: [['', '—'], ['M', 'Male'], ['F', 'Female']] },
      { name: 'parent_name', label: 'Parent name', required: true }, { name: 'phone', label: 'Mobile' },
      { name: 'email', label: 'Email', type: 'email' }, { name: 'previous_school', label: 'Previous school' },
      { name: 'status', label: 'Status', type: 'select', options: ADM.filter((a) => a[0] !== 'enrolled') }, { name: 'follow_up', label: 'Follow-up date', type: 'date' },
      { name: 'notes', label: 'Notes', type: 'textarea', rows: 3, full: true },
    ], r),
    onSubmit: async (v) => { r.id ? await PUT('/api/admissions/' + r.id, v) : await POST('/api/admissions', v); toast('Saved', 'success'); load(); },
  });
  const enrol = async (r) => {
    await loadClasses(true);
    modal({
      title: 'Enrol ' + r.child_name, submit: 'Enrol student',
      body: formFields([
        { name: 'class_id', label: 'Class', type: 'select', required: true, full: true, options: classOptions() },
        { name: 'parent_email', label: 'Parent login email', type: 'email', required: true, value: r.email, full: true, hint: 'If a parent account with this email exists (e.g. a sibling), the child is linked to it.' },
        { name: 'parent_password', label: 'Parent password (new accounts)', value: '' },
        { name: 'welcome', label: 'Send a welcome message to the parent', type: 'checkbox', value: true },
      ]),
      onSubmit: async (v) => { await POST(`/api/admissions/${r.id}/enrol`, { ...v, class_id: Number(v.class_id) }); toast(r.child_name + ' enrolled', 'success'); load(); },
    });
  };
  $$('#at button', el).forEach((b) => b.onclick = () => { status = b.dataset.v; $$('#at button', el).forEach((x) => x.classList.toggle('active', x === b)); load(); });
  let t; $('#aq', el).oninput = () => { clearTimeout(t); t = setTimeout(load, 250); };
  $('#add', el).onclick = () => form();
  await load();
};

/* ---------- Library ---------- */
PAGES.library = async (el) => {
  el.innerHTML = `<div class="row" style="margin-bottom:16px"><div class="tabs" id="lbt"><button class="active" data-v="loans">Loans</button><button data-v="books">Books</button></div><span class="spacer"></span>
    <button class="btn" id="remind">${icon('bell')} Remind overdue</button><button class="btn primary" id="issue">${icon('plus')} Issue book</button></div><div id="lb">${loading()}</div>`;
  const box = $('#lb', el);
  let tab = 'loans';
  const loans = async () => {
    box.innerHTML = `<div class="card"><div class="card-h"><div class="tabs" id="ls">${[['active', 'On loan'], ['overdue', 'Overdue'], ['returned', 'Returned'], ['', 'All']].map(([v, l], i) => `<button data-v="${v}" class="${i === 0 ? 'active' : ''}">${l}</button>`).join('')}</div></div><div id="ll">${loading()}</div></div>`;
    let st = 'active';
    const load = async () => {
      const rows = await GET('/api/loans?status=' + st);
      $('#ll', box).innerHTML = table([
        { label: 'Book', render: (r) => `<b>${esc(r.title)}</b>` }, { label: 'Student', render: (r) => `${esc(r.student_name)}<div class="muted small">${esc(r.admission_no)} · ${esc(r.class_name || '')}</div>` },
        { label: 'Issued', render: (r) => fmtDate(r.issued_at) }, { label: 'Due', render: (r) => r.overdue ? `<b style="color:var(--danger)">${fmtDate(r.due_date)}</b>` : fmtDate(r.due_date) },
        { label: 'Status', render: (r) => r.returned_at ? badge('returned', 'Returned ' + fmtDate(r.returned_at)) : r.overdue ? badge('overdue') : badge('active', 'On loan') },
        { label: '', render: (r) => r.returned_at ? '' : `<button class="btn sm success" data-ret="${r.id}">${icon('check')} Returned</button>` },
      ], rows, 'No loans.');
      $$('[data-ret]', box).forEach((b) => b.onclick = () => POST(`/api/loans/${b.dataset.ret}/return`).then(() => { toast('Book returned', 'success'); load(); }).catch((e) => toast(e.message, 'error')));
    };
    $$('#ls button', box).forEach((b) => b.onclick = () => { st = b.dataset.v; $$('#ls button', box).forEach((x) => x.classList.toggle('active', x === b)); load(); });
    await load();
  };
  const books = async () => {
    box.innerHTML = `<div class="card"><div class="card-h"><input class="input" id="bq" placeholder="Search title, author, ISBN…" style="max-width:280px"><span class="spacer"></span><button class="btn" id="addB">${icon('plus')} Add book</button></div><div id="bl">${loading()}</div></div>`;
    const load = async () => {
      const rows = await GET('/api/books?q=' + encodeURIComponent($('#bq', box).value));
      $('#bl', box).innerHTML = table([
        { label: 'Title', render: (r) => `<b>${esc(r.title)}</b><div class="muted small">${esc(r.author)}</div>` }, { label: 'Category', key: 'category' }, { label: 'ISBN', key: 'isbn' },
        { label: 'Available', num: true, render: (r) => `<b style="color:${r.available ? 'inherit' : 'var(--danger)'}">${r.available}</b> / ${r.copies}` },
        { label: '', render: (r) => `<div class="row" style="justify-content:flex-end"><button class="btn sm" data-be="${r.id}">${icon('edit')}</button><button class="btn sm danger" data-bd="${r.id}">${icon('trash')}</button></div>` },
      ], rows, 'No books yet.');
      $$('[data-be]', box).forEach((b) => b.onclick = () => form(rows.find((r) => r.id == b.dataset.be)));
      $$('[data-bd]', box).forEach((b) => b.onclick = async () => { if (await confirmBox('Delete this book and its loan history?', 'Delete')) DEL('/api/books/' + b.dataset.bd).then(load).catch((e) => toast(e.message, 'error')); });
    };
    const form = (b = {}) => modal({ title: b.id ? 'Edit book' : 'Add book', body: formFields([{ name: 'title', label: 'Title', required: true, full: true }, { name: 'author', label: 'Author' }, { name: 'category', label: 'Category' }, { name: 'isbn', label: 'ISBN' }, { name: 'copies', label: 'Copies', type: 'number', min: 1, value: b.copies || 1 }], b),
      onSubmit: async (v) => { b.id ? await PUT('/api/books/' + b.id, v) : await POST('/api/books', v); toast('Book saved', 'success'); load(); } });
    $('#addB', box).onclick = () => form();
    let t; $('#bq', box).oninput = () => { clearTimeout(t); t = setTimeout(load, 250); };
    await load();
  };
  $$('#lbt button', el).forEach((b) => b.onclick = () => { tab = b.dataset.v; $$('#lbt button', el).forEach((x) => x.classList.toggle('active', x === b)); (tab === 'loans' ? loans : books)(); });
  $('#remind', el).onclick = async () => { try { const r = await POST('/api/loans/remind-overdue'); toast(`${r.reminded} overdue reminder(s) sent to parents`, 'success'); } catch (e) { toast(e.message, 'error'); } };
  $('#issue', el).onclick = async () => {
    const [bk, st] = await Promise.all([GET('/api/books'), GET('/api/students')]);
    modal({ title: 'Issue book', submit: 'Issue',
      body: formFields([
        { name: 'book_id', label: 'Book', type: 'select', full: true, options: bk.filter((b) => b.available > 0).map((b) => [b.id, `${b.title} (${b.available} available)`]) },
        { name: 'student_id', label: 'Student', type: 'select', full: true, options: st.map((s) => [s.id, `${s.name} · ${s.admission_no} · ${s.class_name || ''}`]) },
        { name: 'due_date', label: 'Due date', type: 'date', value: addDays(14) },
      ]),
      onSubmit: async (v) => { await POST('/api/loans', { book_id: Number(v.book_id), student_id: Number(v.student_id), due_date: v.due_date }); toast('Book issued', 'success'); if (tab === 'loans') loans(); } });
  };
  await loans();
};
