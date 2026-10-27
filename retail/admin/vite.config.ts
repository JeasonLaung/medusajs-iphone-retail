import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Isolated live rehearsals can choose an owned, ephemeral loopback API port.
// No browser-controlled URL or remote target is accepted.
const apiTarget = process.env.ADMIN_TEST_API_TARGET ?? 'http://127.0.0.1:9100'
const parsed = new URL(apiTarget)
if (parsed.protocol !== 'http:' || parsed.hostname !== '127.0.0.1' || !parsed.port || parsed.pathname !== '/' || parsed.search || parsed.hash || parsed.username || parsed.password) {
  throw new Error('Admin API proxy requires an explicit loopback HTTP port')
}

export default defineConfig({
  plugins: [react()],
  server: {
    host: '127.0.0.1', port: 9200, strictPort: true,
    proxy: { '/api': { target: apiTarget, changeOrigin: false } },
  },
})
