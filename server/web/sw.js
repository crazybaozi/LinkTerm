var CACHE_NAME = 'linkterm-v8';
var SHELL_FILES = [
    '/',
    '/terminal.html',
    '/css/style.css',
    '/js/auth.js',
    '/js/terminal.js',
    '/manifest.json'
];

self.addEventListener('install', function(e) {
    e.waitUntil(
        caches.open(CACHE_NAME).then(function(cache) {
            return cache.addAll(SHELL_FILES);
        })
    );
    self.skipWaiting();
});

self.addEventListener('activate', function(e) {
    e.waitUntil(
        caches.keys().then(function(names) {
            return Promise.all(
                names.filter(function(name) { return name !== CACHE_NAME; })
                     .map(function(name) { return caches.delete(name); })
            );
        })
    );
    self.clients.claim();
});

self.addEventListener('fetch', function(e) {
    var url = e.request.url;
    if (url.includes('/ws/') || url.includes('/api/') || url.includes('/health/')) {
        return;
    }
    e.respondWith(
        fetch(e.request).then(function(resp) {
            if (resp && resp.status === 200) {
                var clone = resp.clone();
                caches.open(CACHE_NAME).then(function(cache) {
                    cache.put(e.request, clone);
                });
            }
            return resp;
        }).catch(function() {
            return caches.match(e.request);
        })
    );
});
