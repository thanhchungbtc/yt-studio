import { fileURLToPath, URL } from 'node:url'

import { defineConfig } from 'vite'
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
        // The operator-supplied backdrop and typefaces the thumbnail editor
        // draws with. Without this the SPA fallback answers with index.html at
        // 200, which fails to parse as a font and leaves the editor composing
        // in a substitute face against no background.
        '/resources/': { target: apiTarget, changeOrigin: true },
        '/events': {
          target: apiTarget,
          changeOrigin: true,
          // Server-sent events must not be buffered by the proxy.
          configure: (proxy) => {
            proxy.on('proxyRes', (proxyRes) => {
              proxyRes.headers['cache-control'] = 'no-cache, no-transform'
            })
          },
        },
      },
    },
    build: {
      outDir: 'dist',
      emptyOutDir: true,
      // Not the default 'assets': that path belongs to yt-studio's
      // content-addressed artifact route, and a bundle filename would be read as
      // a content address and 404.
      assetsDir: 'app',
      // The operator console is not on the critical path to first paint, so it is
      // lazily imported by the router and lands in its own chunk. Rollup's
      // default vendor splitting is left alone: hand-written groups produced a
      // circular chunk here for no measurable gain.
      reportCompressedSize: true,
      chunkSizeWarningLimit: 300,
    },
  }
})
