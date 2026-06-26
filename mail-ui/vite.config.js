import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Default config: standalone demo/dev SPA build (dist/).
// For the redistributable library build use vite.config.lib.js.
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setup.js'],
    include: ['src/**/*.test.{js,jsx}'],
  },
})
