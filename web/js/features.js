/* Pickup changes and progress reports (staff console and parent app) */
'use strict';

/* ---------- Progress report (shared) ---------- */
async function progressView(el, sid, opts = {}) {
  el.innerHTML = loading();
  let d = await GET(`/api/progress/${sid}${opts.ai ? '?ai=1' : ''}`);
  const att = d.attendance;
  const arrow = (now, before) => before == null || before < 0 ? '' : now > before + 0.5 ? `<span style="color:var(--success)">▲</span>` : now < before - 0.5 ? `<span style="color:var(--danger)">▼</span>` : '<span class="muted">▬</span>';
  const bar = (v, color) => `<div class="track"><div class="fill" style="width:${Math.max(2, Math.min(100, v))}%;background:${color}"></div></div>`;
  const colorFor = (v) => v >= 75 ? 'var(--success)' : v >= 50 ? 'var(--orange)' : 'var(--danger)';
  el.innerHTML = `<div class="stack progress-report">
    <div class="card card-b row" style="align-items:flex-start">
      <div class="avatar" style="width:52px;height:52px;font-size:18px">${esc(initials(d.student.name))}</div>
      <div class="grow" style="min-width:0"><h3 style="font-size:18px">${esc(d.student.name)}</h3>
        <div class="muted small">${esc(d.student.class_name || '')} · ${esc(d.student.admission_no)}${d.student.class_teacher ? ` · <span>Class teacher</span>: ${esc(d.student.class_teacher)}` : ''}</div>
        <div class="muted small">${fmtDate(d.generated)}</div></div>
      <button class="btn sm no-print" onclick="window.print()">${icon('diary')} Print</button>
    </div>
    <div class="card"><div class="card-h"><h3>Summary</h3><span class="spacer"></span>
      ${d.summary_source === 'ai' ? `<span class="badge" style="background:linear-gradient(135deg,#7c3aed,#db2777);color:#fff">${icon('sparkle')} Epoch AI</span>` : d.ai_available ? `<button class="btn sm ai no-print" id="aiSum">${icon('sparkle')} Write with Epoch AI</button>` : ''}</div>
      <div class="card-b"><p data-noi18n style="margin:0;line-height:1.7;white-space:pre-line">${esc(d.summary)}</p>${d.ai_error ? `<p class="small" style="color:var(--danger)">${esc(d.ai_error)}</p>` : ''}</div></div>
    <div class="grid g-2">
      <div class="card"><div class="card-h"><h3>Attendance</h3><span class="muted small">last 30 days</span></div><div class="card-b">
        ${att.percent < 0 ? '<div class="muted">No attendance recorded yet.</div>' : `<div class="row" style="flex-wrap:nowrap"><div style="font-size:34px;font-weight:800">${att.percent}%</div><div style="font-size:20px">${arrow(att.percent, att.previous)}</div>
          <div class="muted small">${att.previous >= 0 ? `<span>Previous 30 days</span>: ${att.previous}%` : ''}</div></div>
          <div class="hbar" style="grid-template-columns:1fr"><div>${bar(att.percent, colorFor(att.percent))}</div></div>
          <div class="row small"><span class="badge red">${t('Absent')} ${att.absent}</span><span class="badge amber">${t('Late')} ${att.late}</span></div>`}
      </div></div>
      <div class="card"><div class="card-h"><h3>Next steps</h3></div><div class="card-b">
        <ol data-noi18n style="margin:0;padding-left:20px;line-height:1.7">${d.steps.map((s) => `<li>${esc(s)}</li>`).join('')}</ol></div></div>
    </div>
    <div class="card"><div class="card-h"><h3>Subjects</h3><span class="muted small">latest published results</span></div>
      ${d.subjects.length ? `<div class="card-b">${d.subjects.map((s) => `<div class="subj-row">
        <div><b>${esc(s.subject)}</b><div class="muted small">${esc(s.exam)} · ${fmtDate(s.date)}</div></div>
        <div>${bar(s.latest, colorFor(s.latest))}<div class="muted small" style="margin-top:3px"><span>Class average</span> ${s.class_avg}%${s.previous != null ? ` · <span>Previous</span> ${s.previous}%` : ''}</div></div>
        <div class="num"><b style="font-size:17px">${s.latest}%</b> ${arrow(s.latest, s.previous)}<div><span class="badge blue">${esc(s.grade)}</span></div></div>
      </div>`).join('')}</div>` : '<div class="empty">No published results yet.</div>'}
    </div>
    <div class="grid g-2">
      <div class="card"><div class="card-h"><h3>Strengths</h3></div><div class="card-b">${d.strengths.length ? d.strengths.map((s) => `<span class="badge green" style="margin:2px">${esc(s)}</span>`).join('') : '<span class="muted small">Not enough results yet.</span>'}</div></div>
      <div class="card"><div class="card-h"><h3>Needs support</h3></div><div class="card-b">${d.focus.length ? d.focus.map((s) => `<span class="badge red" style="margin:2px">${esc(s)}</span>`).join('') : '<span class="muted small">Nothing flagged — well done.</span>'}</div></div>
    </div>
    <div class="grid g-2">
      <div class="card"><div class="card-h"><h3>Teacher remarks</h3></div>${d.remarks.length ? d.remarks.map((r) => `<div class="list-item">${catIcon('results')}<div class="grow"><div class="t1">${esc(r.subject)} · ${esc(r.exam)}</div><div class="t2" data-noi18n>“${esc(r.remark)}”</div></div></div>`).join('') : '<div class="empty">No remarks yet.</div>'}</div>
      <div class="card"><div class="card-h"><h3>Coming up</h3></div>${d.homework.length ? d.homework.map((h) => `<div class="list-item">${catIcon('homework')}<div class="grow"><div class="t1">${esc(h.subject)} · <span data-noi18n>${esc(h.title)}</span></div><div class="t2">${t('Due')}: ${fmtDate(h.due_date)}</div></div></div>`).join('') : '<div class="empty">No homework due.</div>'}</div>
    </div>
  </div>`;
  const ai = $('#aiSum', el);
  ai && (ai.onclick = async () => { ai.disabled = true; ai.innerHTML = `${icon('sparkle')} …`; await progressView(el, sid, { ai: true }); });
}

/* ---------- Staff: progress page and pickup changes ---------- */
PAGES.progress = async (el) => {
  const sid = location.hash.split('/')[2];
  if (!sid) { location.hash = '#/students'; return; }
  await progressView(el, sid);
};

const PICKUP_KINDS = [['collector', 'Someone else will collect'], ['no_bus', 'Not taking the school bus'], ['early', 'Early pickup'], ['other', 'Other change']];
const pickupText = (p) => p.kind === 'collector' ? `<b data-noi18n>${esc(p.collector_name)}</b> (${esc(p.collector_relation || '—')}) · <span data-noi18n>${esc(p.collector_phone)}</span>`
  : p.kind === 'early' ? `${t('Early pickup')} · <b>${esc(p.pickup_time)}</b>` : p.kind === 'no_bus' ? t('Not taking the school bus') : '';

PAGES.pickups = async (el) => {
  el.innerHTML = `<div class="card"><div class="card-h"><input class="input" type="date" id="pd" value="${todayStr()}" style="width:auto">
    <span class="muted small">Changes sent by parents from the app. Check the collector's ID at the gate.</span></div><div id="pl">${loading()}</div></div>`;
  const load = async () => {
    const rows = await GET('/api/pickups?date=' + $('#pd', el).value);
    $('#pl', el).innerHTML = table([
      { label: 'Student', render: (r) => `<b>${esc(r.student_name)}</b><div class="muted small">${esc(r.class_name || '')}${r.route_name ? ' · ' + esc(r.bus_no || r.route_name) : ''}</div>` },
      { label: 'Change', render: (r) => `<div>${esc(t((PICKUP_KINDS.find((k) => k[0] === r.kind) || [, r.kind])[1]))}</div><div class="small">${pickupText(r)}</div>${r.note ? `<div class="muted small" data-noi18n>“${esc(r.note)}”</div>` : ''}` },
      { label: 'Parent', render: (r) => `${esc(r.parent_name || '')}<div class="muted small">${esc(r.parent_phone || '')}</div>` },
      { label: 'Status', render: (r) => badge(r.status === 'acknowledged' ? 'approved' : r.status === 'declined' ? 'rejected' : 'pending', cap(r.status === 'acknowledged' ? 'confirmed' : r.status)) + (r.handled_by_name ? `<div class="muted small">${esc(r.handled_by_name)}</div>` : '') },
      { label: '', render: (r) => r.status === 'pending' ? `<div class="row" style="flex-wrap:nowrap;justify-content:flex-end"><button class="btn sm success" data-ok="${r.id}">${icon('check')} Confirm</button><button class="btn sm danger" data-no="${r.id}">${icon('x')}</button></div>` : '' },
    ], rows, 'No pickup changes for this day.');
    const respond = (id, status) => modal({
      title: status === 'acknowledged' ? 'Confirm pickup change' : 'Decline pickup change', submit: status === 'acknowledged' ? 'Confirm' : 'Decline',
      body: field({ name: 'note', label: 'Message to the parent (optional)', type: 'textarea', rows: 2 }),
      onSubmit: async (v) => { await POST(`/api/pickups/${id}/respond`, { status, note: v.note }); toast(t('Parent notified'), 'success'); S.pendingPickups = Math.max(0, (S.pendingPickups || 1) - 1); load(); },
    });
    $$('[data-ok]', el).forEach((b) => b.onclick = () => respond(b.dataset.ok, 'acknowledged'));
    $$('[data-no]', el).forEach((b) => b.onclick = () => respond(b.dataset.no, 'declined'));
  };
  $('#pd', el).onchange = load;
  await load();
};

/* ---------- Parent app: progress and pickup ---------- */
PV.progress = async (el, d) => { await progressView(el, d.student.id); };

PV.pickup = async (el, d) => {
  const st = d.student;
  el.innerHTML = `<form class="card card-b" id="pkf"><h3 style="margin-bottom:6px">Pickup change for ${esc(st.name.split(' ')[0])}</h3>
      <p class="muted small" style="margin-top:0">Tell the school if someone else is collecting, your child is not taking the bus, or you will collect early.</p>
      ${field({ name: 'date', label: 'Date', type: 'date', required: true, value: todayStr() })}
      ${field({ name: 'kind', label: 'What is changing?', type: 'select', options: PICKUP_KINDS })}
      <div id="pkx"></div>
      ${field({ name: 'note', label: 'Note to the school (optional)', type: 'textarea', rows: 2 })}
      <button class="btn primary block">${icon('send')} Send to school</button></form>
    <div class="card"><div class="card-h"><h3>My pickup changes</h3></div><div id="pkl">${loading()}</div></div>`;
  const form = $('#pkf', el), extra = $('#pkx', el);
  const showExtra = () => {
    const k = form.elements.kind.value;
    extra.innerHTML = k === 'collector' ? field({ name: 'collector_name', label: 'Name of the person collecting', required: true }) + field({ name: 'collector_relation', label: 'Relationship', placeholder: 'Grandfather, aunt, driver…' }) + field({ name: 'collector_phone', label: 'Their mobile number', required: true, placeholder: '07X XXX XXXX' })
      : k === 'early' ? field({ name: 'pickup_time', label: 'Pickup time', type: 'time', required: true, value: '11:30' }) : '';
    if (LANG !== 'en') translateNode(extra);
  };
  form.elements.kind.onchange = showExtra; showExtra();
  const loadList = async () => {
    const rows = (await GET('/api/pickups')).filter((r) => r.student_id === st.id);
    $('#pkl', el).innerHTML = rows.length ? rows.map((r) => `<div class="list-item">${catIcon('transport')}<div class="grow">
      <div class="t1">${fmtDate(r.date)} · ${esc(t((PICKUP_KINDS.find((k) => k[0] === r.kind) || [, r.kind])[1]))}</div><div class="t2">${pickupText(r)}${r.response ? ` · <span data-noi18n>“${esc(r.response)}”</span>` : ''}</div></div>
      <div style="text-align:right">${badge(r.status === 'acknowledged' ? 'approved' : r.status === 'declined' ? 'rejected' : r.status === 'cancelled' ? 'cancelled' : 'pending', cap(r.status === 'acknowledged' ? 'confirmed' : r.status))}
      ${(r.status === 'pending' || r.status === 'acknowledged') && r.date >= todayStr() ? `<div><button class="btn sm ghost" data-c="${r.id}">Cancel</button></div>` : ''}</div></div>`).join('') : '<div class="empty">No pickup changes yet.</div>';
    $$('[data-c]', el).forEach((b) => b.onclick = async () => { if (await confirmBox(t('Cancel this pickup change?'), t('Cancel request'))) POST(`/api/pickups/${b.dataset.c}/cancel`).then(loadList).catch((e) => toast(e.message, 'error')); });
  };
  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    const v = readForm(form);
    try { await POST('/api/pickups', { ...v, student_id: st.id }); toast(t('Sent to school — you will get a reply here'), 'success'); form.reset(); form.elements.date.value = todayStr(); showExtra(); loadList(); }
    catch (err) { toast(err.message, 'error'); }
  });
  await loadList();
};
