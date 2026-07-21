import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileViewerRenderers } from '@file-viewer/vite-plugin'
import path from 'path'

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    vue(),
    fileViewerRenderers({ copyAssets: true }),
  ],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, 'src'),
    },
  },
  build: {
    rollupOptions: {
      output: {
        manualChunks(id: string) {
          if (id.includes('node_modules/vue') || id.includes('node_modules/pinia') || id.includes('node_modules/vue-router')) return 'vendor-vue'
          if (id.includes('node_modules/ant-design-vue')) return 'vendor-ui'
          if (id.includes('node_modules/echarts')) return 'vendor-charts'
          if (id.includes('node_modules/mermaid')) return 'vendor-diagram'
          if (id.includes('node_modules/katex') || id.includes('node_modules/markdown-it')) return 'vendor-markdown'
          if (id.includes('node_modules/@vue-flow')) return 'vendor-flow'
        },
      },
    },
  },
})
