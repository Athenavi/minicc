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
})
