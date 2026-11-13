/// <reference types="vitest" />
import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'path';

export default defineConfig(({ mode }) => {
  // Server-side only: never prefixed with VITE_, so it stays out of the bundle.
  const env = loadEnv(mode, process.cwd(), '');
  const staleBackend = env.STALE_BACKEND_URL || 'http://localhost:8000';
  const staleApiKey = env.STALE_API_KEY || '';
  const proxyHeaders: Record<string, string> = staleApiKey ? { 'X-API-Key': staleApiKey } : {};

  return {
    plugins: [react()],
    resolve: {
      alias: {
        '@': path.resolve(__dirname, './src'),
      },
    },
    server: {
      port: 3000,
      proxy: {
        '/v1': {
          target: staleBackend,
          changeOrigin: true,
          headers: proxyHeaders,
        },
      },
    },
    test: {
      include: ['src/**/*.{test,spec}.{js,mjs,cjs,ts,mts,cts,jsx,tsx}'],
      exclude: ['**/node_modules/**', '**/dist/**', '**/e2e/**'],
    },
    build: {
      rollupOptions: {
        output: {
          manualChunks(id) {
            if (id.includes('node_modules/recharts')) {
              return 'vendor-charts';
            }
          },
        },
      },
    },
  };
});
