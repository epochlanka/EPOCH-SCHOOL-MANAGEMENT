/* Installable web app: lets parents, students and staff add Epoch School to their
   phone's home screen without the Play Store or App Store. Adds "Install app" buttons
   to the sign-in page, the staff top bar and the parent app's Profile page by itself. */
'use strict';

(() => {
  let prompt = null;
  const installed = () => matchMedia('(display-mode: standalone)').matches || navigator.standalone === true;
  const ios = () => /iphone|ipad|ipod/i.test(navigator.userAgent) || (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1);

  Object.assign(DICT, {
    'Install app': ['යෙදුම ස්ථාපනය කරන්න', 'செயலியை நிறுவு'],
    'Install Epoch School': ['Epoch School ස්ථාපනය කරන්න', 'Epoch School ஐ நிறுவு'],
    'Get the app': ['යෙදුම ලබා ගන්න', 'செயலியைப் பெறுக'],
    'Add Epoch School to your home screen for one-tap access.': ['එක් තට්ටුවකින් විවෘත කිරීමට Epoch School ඔබේ මුල් තිරයට එක් කරන්න.', 'ஒரே தட்டலில் திறக்க Epoch School ஐ உங்கள் முகப்புத் திரையில் சேர்க்கவும்.'],
    'Epoch School installed on this phone': ['Epoch School මෙම දුරකථනයේ ස්ථාපනය විය', 'Epoch School இந்தத் தொலைபேசியில் நிறுவப்பட்டது'],
    'Tap the Share button in Safari.': ['Safari හි Share බොත්තම ඔබන්න.', 'Safari இல் Share பொத்தானைத் தட்டவும்.'],
    'Choose "Add to Home Screen", then "Add".': ['"Add to Home Screen" තෝරා, "Add" ඔබන්න.', '"Add to Home Screen" ஐத் தேர்ந்தெடுத்து "Add" தட்டவும்.'],
    'Open the browser menu ⋮ at the top right.': ['ඉහළ දකුණේ ඇති බ්‍රවුසර මෙනුව ⋮ විවෘත කරන්න.', 'மேல் வலதுபுறத்தில் உள்ள உலாவி மெனு ⋮ ஐத் திறக்கவும்.'],
    'Choose "Install app" or "Add to Home screen".': ['"Install app" හෝ "Add to Home screen" තෝරන්න.', '"Install app" அல்லது "Add to Home screen" ஐத் தேர்ந்தெடுக்கவும்.'],
    'The Epoch School icon appears on your home screen.': ['Epoch School අයිකනය ඔබේ මුල් තිරයේ දිස් වේ.', 'Epoch School ஐகான் உங்கள் முகப்புத் திரையில் தோன்றும்.'],
  });

  window.addEventListener('beforeinstallprompt', (e) => { e.preventDefault(); prompt = e; });
  window.addEventListener('appinstalled', () => { prompt = null; document.querySelectorAll('.pwa-install').forEach((b) => b.remove()); toast(t('Epoch School installed on this phone'), 'success'); });

  window.installApp = async () => {
    if (prompt) { prompt.prompt(); const { outcome } = await prompt.userChoice; if (outcome === 'accepted') prompt = null; return; }
    const steps = ios()
      ? ['Tap the Share button in Safari.', 'Choose "Add to Home Screen", then "Add".', 'The Epoch School icon appears on your home screen.']
      : ['Open the browser menu ⋮ at the top right.', 'Choose "Install app" or "Add to Home screen".', 'The Epoch School icon appears on your home screen.'];
    modal({ title: t('Install Epoch School'), footer: false, body: `<ol style="padding-left:20px;line-height:2;margin:0">${steps.map((s) => `<li>${esc(t(s))}</li>`).join('')}</ol>` });
  };

  const btn = (cls, text = true) => {
    const b = document.createElement('button');
    b.type = 'button'; b.className = cls + ' pwa-install'; b.onclick = window.installApp;
    b.innerHTML = icon('phone') + (text ? ' ' + esc(t('Install app')) : '');
    if (!text) { b.title = t('Install app'); b.setAttribute('aria-label', t('Install app')); }
    return b;
  };
  // Add buttons whenever the relevant screens are drawn.
  const decorate = () => {
    if (installed()) return;
    const login = document.querySelector('#loginForm');
    if (login && !login.querySelector('.pwa-install')) { const w = document.createElement('div'); w.style.cssText = 'text-align:center;margin-top:8px'; w.append(btn('btn ghost')); login.append(w); }
    const theme = document.querySelector('.topbar #themeBtn');
    if (theme && !document.querySelector('.topbar .pwa-install')) theme.before(btn('icon-btn', false));
    const pf = document.querySelector('#pf');
    if (pf && !document.querySelector('.pwa-card')) {
      const c = document.createElement('div'); c.className = 'card card-b row pwa-card';
      c.innerHTML = `<div class="ic tint-blue" style="width:44px;height:44px;border-radius:12px;display:grid;place-items:center">${icon('phone')}</div><div class="grow"><b>${esc(t('Get the app'))}</b><div class="muted small">${esc(t('Add Epoch School to your home screen for one-tap access.'))}</div></div>`;
      c.append(btn('btn primary')); pf.before(c);
    }
  };
  new MutationObserver(decorate).observe(document.documentElement, { childList: true, subtree: true });

  const style = document.createElement('style');
  style.textContent = `@media (display-mode: standalone) { .p-head { padding-top: calc(18px + env(safe-area-inset-top)); } .topbar { padding-top: calc(14px + env(safe-area-inset-top)); } }`;
  document.head.append(style);

  // The service worker needs a secure address (https) or localhost.
  if ('serviceWorker' in navigator && (location.protocol === 'https:' || location.hostname === 'localhost')) {
    navigator.serviceWorker.register('/sw.js').catch((e) => console.warn('Service worker not registered:', e));
  }
})();
