import { writeFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import { defineConfig, type Plugin } from 'vitest/config'

// floe built with the `dev` tag listens here. This server proxies /api to it;
// a release build has no fixed port and answers no other origin.
const floe = 'http://127.0.0.1:5174'

// Vite empties dist/ on every build, and that would take the placeholder which
// keeps `go build` working before the first frontend build. Put it back.
function keepPlaceholder(): Plugin {
  return {
    name: 'floe-keep-dist-placeholder',
    apply: 'build',
    writeBundle() {
      writeFileSync(resolve(import.meta.dirname, 'dist/.gitkeep'), '')
    },
  }
}

export default defineConfig({
  plugins: [svelte(), keepPlaceholder()],
  server: {
    port: 5173,
    strictPort: true,
    // The Host header reaches floe as localhost:5173, which a dev build
    // answers alongside its own address, so changeOrigin is left off.
    proxy: { '/api': floe },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    // One page, embedded and served from a local binary: no code splitting to
    // win, and a source map would only bloat what go:embed carries.
    sourcemap: false,
  },
  test: {
    include: ['src/**/*.test.ts'],
    environment: 'node',
  },
})
