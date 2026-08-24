import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileViewerRenderers } from '@file-viewer/vite-plugin'
import Components from 'unplugin-vue-components/vite'
import { AntDesignVueResolver } from 'unplugin-vue-components/resolvers'
import path from 'path'

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    vue(),
    fileViewerRenderers({ copyAssets: true }),
    // antd 按需引入：只打包模板中实际使用的 a-* 组件，显著削减首屏体积。
    // 组件样式由 antd v4 cssinjs 运行时注入，无需额外 style 导入。
    Components({
      resolvers: [AntDesignVueResolver({ importStyle: false })],
      dts: 'src/components.d.ts',
    }),
  ],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, 'src'),
    },
    // 强制 Vue 单实例：pnpm 为 ant-design-vue/@ant-design/icons-vue 解析到
    // node_modules/.pnpm 下的 vue@3.5.41，与 app 的 vue@3.5.39 不一致，
    // 导致 provide/inject 上下文断裂（prefixCls undefined）
    dedupe: ['vue'],
  },
  server: {
    // 代理后端 API：避免开发时 CORS 跨域问题（后端 CORS_ORIGINS 仅含 5173）
    proxy: {
      '/v1': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      '/events': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      '/ws': {
        target: 'ws://localhost:8080',
        ws: true,
      },
      '/metrics': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      // S 修复：/submit /cancel /media 为网关非 /v1 前缀路由，dev 亦需代理
      '/submit': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      '/cancel': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      '/media': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
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
