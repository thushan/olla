import { writeFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from '@tailwindcss/vite';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

// Vite writes straight into the go:embed source (see internal/app/handlers/
// dashboard/embed.go) so `make build-web` no longer needs a separate
// rm/cp/touch copy step - one less moving part, and it matches how the other
// web targets (install-web, test-web, lint-web) already run a bare npm
// command. emptyOutDir wipes the directory (including the committed
// .gitkeep sentinel) on every build, so it is rewritten by the plugin below.
const EMBED_DIST = path.resolve(__dirname, '../../internal/app/handlers/dashboard/dist');

// Restores the .gitkeep sentinel emptyOutDir removes.
function restoreGitkeep() {
  return {
    name: 'restore-embed-gitkeep',
    writeBundle() {
      writeFileSync(path.join(EMBED_DIST, '.gitkeep'), '');
    },
  };
}

// Served at /internal/ui/ on the proxy listener (spec FR-1). WP-3 mounts the
// go:embed handler under that subpath, so the SPA must request its hashed
// assets from /internal/ui/assets/... rather than the site root.
export default defineConfig({
  base: '/internal/ui/',
  plugins: [svelte(), tailwindcss(), restoreGitkeep()],
  build: {
    outDir: EMBED_DIST,
    emptyOutDir: true,
    sourcemap: false,
  },
  // Vitest runs under Node, but component tests mount real Svelte components
  // (see SortableTable.test.js) - without this, svelte resolves to its
  // server-only build and `mount()` throws lifecycle_function_unavailable.
  resolve: process.env.VITEST ? { conditions: ['browser'] } : undefined,
  test: {
    environment: 'jsdom',
    include: ['src/**/*.{test,spec}.js'],
  },
});
