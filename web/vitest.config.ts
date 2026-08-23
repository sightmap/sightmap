import { defineConfig } from 'vitest/config'
import { resolve } from 'path'

export default defineConfig({
  resolve: {
    alias: { '@': resolve(__dirname, './src') },
  },
  test: {
    environment: 'node',
    include: ['scripts/**/*.test.ts', 'src/**/*.test.ts', 'src/**/*.test.tsx', 'netlify/**/*.test.ts'],
  },
})
