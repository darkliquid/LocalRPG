import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import path from 'path';

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 3000,
    proxy: {
      '/api': 'http://localhost:8080'
    }
  },
  build: {
    outDir: path.resolve(__dirname, '../pkg/gui/dist'),
    emptyOutDir: true,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes('node_modules')) {
            if (id.includes('@lezer')) {
              return 'vendor-lezer';
            }
            if (id.includes('@codemirror') || id.includes('codemirror')) {
              return 'vendor-codemirror';
            }
            if (
              id.includes('react-markdown') ||
              id.includes('remark-') ||
              id.includes('mdast-') ||
              id.includes('micromark') ||
              id.includes('unist-')
            ) {
              return 'vendor-markdown';
            }
            if (id.includes('lucide-react')) {
              return 'vendor-icons';
            }
            if (id.includes('react') || id.includes('react-dom')) {
              return 'vendor-react';
            }
            if (id.includes('@headless-tree')) {
              return 'vendor-tree';
            }
          }
        }
      }
    }
  }
});
