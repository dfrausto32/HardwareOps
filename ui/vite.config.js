import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const apiBase = env.VITE_API_BASE_URL || 'https://localhost:8080'
  const simulateProd = env.VITE_SIMULATE_PROD === '1'
  const useProxy = env.VITE_API_PROXY === '1' && !simulateProd
  const wsTarget = apiBase.replace(/^http/, 'ws')

  return {
    plugins: [react()],
    server: {
      port: 5173,
      proxy: useProxy
        ? {
            '/api/v1/events': {
              target: wsTarget,
              changeOrigin: true,
              secure: false,
              ws: true,
            },
            '/api': {
              target: apiBase,
              changeOrigin: true,
              secure: false,
            },
            '/healthz': {
              target: apiBase,
              changeOrigin: true,
              secure: false,
            },
            '/metrics': {
              target: apiBase,
              changeOrigin: true,
              secure: false,
            },
          }
        : undefined,
    },
  }
})
