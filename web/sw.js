/* Epoch School Connect service worker: makes the app installable and lets it open on a
   weak connection. App files come from the network first (so updates show at once) and
   are cached as a fallback. School data (/api/) is never cached. */
const CACHE = 'epoch-app-v1';
const SHELL = [
  '/', '/index.html', '/manifest.json', '/css/app.css',
  '/js/i18n.js', '/js/ui.js', '/js/staff.js', '/js/staff2.js', '/js/portal.js', '/js/features.js', '/js/main.js', '/js/pwa.js',
  '/assets/logo-mark.png', '/assets/logo-full.png', '/assets/icon-192.png', '/assets/favicon.png',
];

self.addEventListener('install', (e) => {
  e.waitUntil(caches.open(CACHE).then((c) => c.addAll(SHELL)).then(() => self.skipWaiting()));
});

self.addEventListener('activate', (e) => {
  e.waitUntil(caches.keys()
    .then((keys) => Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k))))
    .then(() => self.clients.claim()));
});

self.addEventListener('fetch', (e) => {
  const url = new URL(e.request.url);
  if (e.request.method !== 'GET' || url.origin !== location.origin || url.pathname.startsWith('/api/')) return;
  e.respondWith(
    fetch(e.request)
      .then((res) => {
        if (res.ok) { const copy = res.clone(); caches.open(CACHE).then((c) => c.put(e.request, copy)); }
        return res;
      })
      .catch(() => caches.match(e.request).then((hit) => hit || (e.request.mode === 'navigate' ? caches.match('/index.html') : Response.error()))),
  );
});
