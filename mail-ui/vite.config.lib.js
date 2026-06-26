/**
 * vite.config.lib.js — library build for @vulos/mail-ui
 *
 * Produces dist-lib/ with ESM + CJS bundles plus a single bundled stylesheet
 * (dist-lib/mail-ui.css). Externalizes react/react-dom so consumers dedupe.
 *
 * Usage: vite build --config vite.config.lib.js
 */

import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { resolve } from 'path'

const dir = import.meta.dirname

export default defineConfig({
  plugins: [react()],
  build: {
    lib: {
      entry: {
        index: resolve(dir, 'src/lib/index.js'),
        api: resolve(dir, 'src/api.js'),
      },
      formats: ['es', 'cjs'],
      cssFileName: 'mail-ui',
      fileName: (format, entryName) =>
        format === 'es' ? `${entryName}.js` : `${entryName}.cjs`,
    },
    outDir: 'dist-lib',
    emptyOutDir: true,
    cssCodeSplit: false,
    sourcemap: false,
    rollupOptions: {
      external: [
        'react',
        'react-dom',
        'react/jsx-runtime',
        'react-dom/client',
      ],
      output: {
        exports: 'named',
        globals: {
          react: 'React',
          'react-dom': 'ReactDOM',
          'react/jsx-runtime': 'ReactJSXRuntime',
        },
      },
    },
  },
})
