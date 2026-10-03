import path from 'node:path'
import { fileURLToPath } from 'node:url'

import { defineConfig, loadEnv } from '@rsbuild/core'
import { pluginReact } from '@rsbuild/plugin-react'
import { pluginTailwindcss } from '@rsbuild/plugin-tailwindcss'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

export default defineConfig(({ envMode }) => {
  const env = loadEnv({ mode: envMode, prefixes: ['VITE_'] })
  const serverUrl =
    process.env.VITE_REACT_APP_SERVER_URL ||
    env.rawPublicVars.VITE_REACT_APP_SERVER_URL ||
    'http://localhost:3000'
  const isProd = envMode === 'production'

  return {
    plugins: [pluginReact(), pluginTailwindcss({ optimize: false })],
    source: { entry: { index: './src/mobile/main.tsx' } },
    resolve: { alias: { '@': path.resolve(__dirname, './src') } },
    html: { template: './src/mobile/index.html' },
    server: {
      host: '0.0.0.0',
      strictPort: false,
      base: '/m',
      proxy: { '/api': { target: serverUrl, changeOrigin: true } },
    },
    output: {
      minify: isProd,
      target: 'web',
      // Assets live under /m so buildMobileShell can strip the prefix and serve
      // them straight from web/mobile-dist.
      assetPrefix: '/m',
      distPath: { root: 'mobile-dist' },
    },
  }
})
