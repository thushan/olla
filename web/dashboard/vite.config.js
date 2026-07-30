import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from '@tailwindcss/vite';

// Served at /internal/ui/ on the proxy listener (spec FR-1). WP-3 mounts the
// go:embed handler under that subpath, so the SPA must request its hashed
// assets from /internal/ui/assets/... rather than the site root.
export default defineConfig({
  base: '/internal/ui/',
  plugins: [svelte(), tailwindcss()],
  build: {
    outDir: 'dist',
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
