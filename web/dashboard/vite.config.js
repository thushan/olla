import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import path from 'path'

// https://vite.dev/config/
export default defineConfig({
  plugins: [svelte()],
  base: '/dashboard/',
  resolve: {
    alias: {
      '$lib': path.resolve('./src/lib')
    }
  },
  server: {
    port: 5173,
    proxy: {
      // Proxy API requests to Olla backend
      '/internal': {
        target: 'http://localhost:40114',
        changeOrigin: true,
      },
      '/olla': {
        target: 'http://localhost:40114',
        changeOrigin: true,
      },
      '/version': {
        target: 'http://localhost:40114',
        changeOrigin: true,
      },
    }
  }
})
