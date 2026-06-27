// Minimal PWA service worker. Its only jobs: satisfy Chrome's installability
// criteria (a fetch handler must exist) and provide an offline fallback page.
// ponytail: network-first, NO precaching of API/WS responses — Multica is a
// live-data app (TanStack Query + WebSocket); aggressive caching would serve
// stale boards. Add a real caching strategy only if offline UX becomes a need.
const CACHE = "multica-shell-v1";
const OFFLINE_URL = "/";

self.addEventListener("install", (event) => {
  event.waitUntil(caches.open(CACHE).then((c) => c.add(OFFLINE_URL)));
  self.skipWaiting();
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) =>
        Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k))),
      ),
  );
  self.clients.claim();
});

self.addEventListener("fetch", (event) => {
  // Only intervene on page navigations; everything else (API, WS, assets)
  // goes straight to the network so data stays fresh.
  if (event.request.mode !== "navigate") return;
  event.respondWith(fetch(event.request).catch(() => caches.match(OFFLINE_URL)));
});
