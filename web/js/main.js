/* Boot, login and routing */
'use strict';

function applyMe(r) {
  S.me = r.user; S.children = r.children; S.school = r.school; S.aiEnabled = r.ai_enabled;
  S.unread = r.unread_notifications; S.unreadMessages = r.unread_messages;
  S.perms = r.perms || []; S.roles = r.roles || []; S.roleLabel = r.role_label; S.isStaff = r.staff; S.teaching = r.teaching;
  S.pendingLeave = r.pending_leave || 0; S.pendingPickups = r.pending_pickups || 0;
  // The account's language wins, unless the user just picked one on the sign-in page.
  const mine = LANG_CODES[r.user.language] || 'en';
  if (pickedOnLogin && mine !== LANG) { pickedOnLogin = false; api('PUT', '/api/me/language', { language: LANG_NAMES[LANG] }).catch(() => {}); r.user.language = LANG_NAMES[LANG]; }
  else if (mine !== LANG) { LANG = mine; try { localStorage.setItem('lang', mine); } catch {} document.documentElement.lang = mine; }
  pickedOnLogin = false;
  document.title = `${r.school.name} · Epoch School Connect`;
}

let pickedOnLogin = false;
function renderLogin() {
  $('#app').innerHTML = `<div class="login">
    <section class="login-hero">
      <div class="hero-brand"><img src="assets/logo-mark.png" alt=""><div><b>EPOCH</b><div style="font-size:12px;color:#9fb2e3">Empowering possibilities through technology</div></div></div>
      <div>
        <h1>School <span>Communication</span><br>& Management</h1>
        <p>Keep parents informed with WhatsApp, SMS and app notifications — attendance alerts, fee reminders, homework, exams, bus updates and more, with an AI assistant built in.</p>
        <div class="hero-chips">
          <span class="hero-chip"><i style="background:#25d366"></i>WhatsApp</span><span class="hero-chip"><i style="background:#e0306a"></i>SMS</span>
          <span class="hero-chip"><i style="background:#3b82f6"></i>App notifications</span><span class="hero-chip"><i style="background:#a78bfa"></i>AI assistant</span>
          <span class="hero-chip"><i style="background:#f59e0b"></i>Fee reminders</span><span class="hero-chip"><i style="background:#2dd4bf"></i>Bus updates</span>
        </div>
      </div>
      <div class="hero-foot">Smart Solutions. Better Tomorrow. · 076 843 7970 · Gongawela Bus Station Complex, Matale</div>
    </section>
    <section class="login-panel"><form class="login-card" id="loginForm">
      <img class="logo-full" src="assets/logo-full.png" alt="Epoch">
      <div style="display:flex;justify-content:center;margin-bottom:18px">${langPicker()}</div>
      <h2>Welcome back</h2><div class="sub">Sign in to your school account</div>
      ${field({ name: 'email', label: 'Email', type: 'email', required: true, autocomplete: 'username', placeholder: 'you@school.lk' })}
      ${field({ name: 'password', label: 'Password', type: 'password', required: true, autocomplete: 'current-password' })}
      <button class="btn primary block" type="submit" style="padding:12px">Sign in</button>
      <p class="muted small" style="text-align:center;margin-top:18px">Admins, teachers, parents and students all sign in here.</p>
    </form></section>
  </div>`;
  $$('.lang-pick button', $('#app')).forEach((b) => b.addEventListener('click', () => { pickedOnLogin = true; }));
  $('#loginForm').addEventListener('submit', async (e) => {
    e.preventDefault();
    const btn = $('button', e.target); btn.disabled = true;
    try { await POST('/api/login', readForm(e.target)); history.replaceState(null, '', '#/'); await boot(); }
    catch (err) { toast(err.message, 'error'); btn.disabled = false; }
  });
}

async function logout() {
  try { await POST('/api/logout'); } catch {}
  S.me = null; S.classes = null; S.classCache = {}; S.routes = null; P.data = null;
  history.replaceState(null, '', location.pathname);
  renderLogin();
}

function route() {
  $$('.modal-bg').forEach((m) => m.remove()); // dialogs belong to the page being left
  if (!S.me) return renderLogin();
  if (S.isStaff) renderStaff();
  else renderPortal().catch((e) => toast(e.message, 'error'));
}

async function refreshCounts() {
  if (!S.me || document.hidden) return;
  try {
    const r = await GET('/api/me');
    const changed = r.unread_notifications !== S.unread || r.unread_messages !== S.unreadMessages;
    applyMe(r);
    if (changed && S.isStaff) {
      const bell = $('#bellBtn'); if (bell) bell.innerHTML = icon('bell') + (S.unread ? `<span class="dot">${S.unread}</span>` : '');
      const link = $('.nav a[href="#/messages"]'); if (link) { const p = $('.pill', link); p && p.remove(); if (S.unreadMessages) link.insertAdjacentHTML('beforeend', `<span class="pill">${S.unreadMessages}</span>`); }
    }
  } catch {}
}

async function boot() {
  try { applyMe(await GET('/api/me')); } catch { S.me = null; }
  route();
}

window.addEventListener('hashchange', route);
setInterval(refreshCounts, 30000);
boot();
