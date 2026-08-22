// ESLint 9+ flat config
import js from '@eslint/js'
import vue from 'eslint-plugin-vue'
import tseslint from 'typescript-eslint'

export default [
  // 全局忽略
  {
    ignores: ['dist/**', 'node_modules/**', 'public/**'],
  },
  // JS 基础规则
  js.configs.recommended,
  // TypeScript 规则
  ...tseslint.configs.recommended,
  // Vue 规则
  ...vue.configs['flat/recommended'],
  // 项目特定配置
  {
    rules: {
      // 允许 console（开发调试用）
      'no-console': 'off',
      // 允许未使用的变量（警告而非错误，避免阻塞构建）
      '@typescript-eslint/no-unused-vars': 'warn',
      // Vue：允许多根节点（Vue 3 支持）
      'vue/multi-word-component-names': 'off',
      // Vue：允许 v-html（经 DOMPurify 净化）
      'vue/no-v-html': 'off',
    },
  },
  // 测试文件配置
  {
    files: ['**/*.spec.ts', '**/*.test.ts'],
    rules: {
      '@typescript-eslint/no-explicit-any': 'off',
    },
  },
]
