import { fileURLToPath, URL } from 'node:url'

import { defineConfig, type ProxyOptions } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// The Vite dev server proxies yt-studio's API and event stream, so `make dev`
// runs the two side by side with no CORS in between. The production build is
// what `go:embed` picks up.
//
// "server" is ambiguous in this file -- Vite has one too -- so the Go one is
// named outright throughout.
export default defineConfig(() => {
  // yt-studio's address comes from the environment variable yt-studio itself
  // reads, and from the same default when it is unset: a port written in two
  // places is a port that will disagree with itself. Read for the proxy target
  // and nothing else -- nothing from here is put in `define`, so no value
  // reaches the bundle.
  const apiTarget = `http://${process.env.YTS_LISTEN || '127.0.0.1:8080'}`

  // A server-sent stream, proxied without buffering. There are two of them —
  // `/events` keeps the cache true, `/llm` carries the model console — and both
  // are long-lived connections that a proxy holding a response would break in
  // the same way: silently, and only once something is slow enough to notice.
  //
  // Every stream the Go server grows needs a line here. Without one the request
  // reaches Vite's SPA fallback instead, which answers index.html at 200 — and
  // an EventSource handed HTML reports a connection error, not a wrong route.
  const sseProxy: ProxyOptions = {
    target: apiTarget,
    changeOrigin: true,
    configure: (proxy) => {
      proxy.on('proxyRes', (proxyRes) => {
        proxyRes.headers['cache-control'] = 'no-cache, no-transform'
      })
    },
  }

  return {
    plugins: [react(), tailwindcss()],
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url)),
      },
    },
    server: {
      // Bind IPv4 explicitly. Vite's default resolves to [::1] only on macOS, so
      // http://127.0.0.1:5173 — which is what yt-studio and the Makefile print —
      // would be refused.
      host: '127.0.0.1',
      port: 5173,
      strictPort: true,
      proxy: {
        '/api': { target: apiTarget, changeOrigin: true },
        '/assets/': { target: apiTarget, changeOrigin: true },
        // The operator-supplied backdrop and typefaces a thumbnail composed in
        // the browser would draw with. Proxied rather than left to the SPA
        // fallback, which answers with index.html at 200 — a font that parses
        // as HTML fails silently and composes in a substitute face.
        '/resources/': { target: apiTarget, changeOrigin: true },
        '/events': sseProxy,
        '/llm': sseProxy,
      },
    },
    build: {
      outDir: 'dist',
      emptyOutDir: true,
      // Not the default 'assets': that path belongs to yt-studio's
      // content-addressed artifact route, and a bundle filename would be read as
      // a content address and 404.
      assetsDir: 'app',
      // Rollup's default vendor splitting is left alone: hand-written groups
      // produced a circular chunk here for no measurable gain.
      reportCompressedSize: true,
      chunkSizeWarningLimit: 300,
    },
  }
})
