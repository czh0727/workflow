import { defineConfig, loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, '.', '')
  const workflowTarget = env.VITE_WORKFLOW_API_TARGET || 'http://localhost:8080'
  const workerTarget = env.VITE_WORKER_API_TARGET || 'http://localhost:8081'
  const temporalTarget = env.VITE_TEMPORAL_UI_TARGET || 'http://localhost:18082'

  return {
    plugins: [vue()],
    server: {
      host: '0.0.0.0',
      port: 5173,
      proxy: {
        '/internal': { target: workflowTarget, changeOrigin: true },
        '/live': { target: workflowTarget, changeOrigin: true },
        '/ready': { target: workflowTarget, changeOrigin: true },
        '/metrics': { target: workflowTarget, changeOrigin: true },
        '/worker': {
          target: workerTarget,
          changeOrigin: true,
          rewrite: (path) => path.replace(/^\/worker/, '')
        },
        '/temporal-ui': {
          target: temporalTarget,
          changeOrigin: true,
          rewrite: (path) => path.replace(/^\/temporal-ui/, '')
        }
      }
    }
  }
})
