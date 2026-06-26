import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import path from 'path'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue()],
  base: '/sqlpanel/',  // 关键：设置基础路径
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    proxy: {
      '/sqlpanel/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
})
