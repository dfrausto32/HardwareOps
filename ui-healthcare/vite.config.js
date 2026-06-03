import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'
import { fileURLToPath } from 'url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const apiBase = env.VITE_API_BASE_URL || 'https://localhost:8080'
  const simulateProd = env.VITE_SIMULATE_PROD === '1'
  const useProxy = env.VITE_API_PROXY === '1' && !simulateProd
  const wsTarget = apiBase.replace(/^http/, 'ws')

  return {
    plugins: [react()],
    resolve: {
      alias: {
        '@shared': path.resolve(__dirname, '../ui-shared/src'),
        '@variant': path.resolve(__dirname, './src/variant.js'),
      },
    },
    server: {
      port: 5174,
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
