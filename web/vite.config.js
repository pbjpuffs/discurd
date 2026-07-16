import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Dev server proxies to the docker stack's Traefik on :80 so `npm run dev`
// works against the real backend (api, gateway, minio) without CORS pain.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:80',
        changeOrigin: true,
      },
      '/files': {
        target: 'http://localhost:80',
        changeOrigin: true,
      },
      '/ws': {
        target: 'http://localhost:80',
        changeOrigin: true,
        ws: true,
      },
    },
  },
})
