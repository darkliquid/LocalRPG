import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import path from 'path';

// The player is built on its own, into its own directory, so it shares no chunks
// or stylesheets with the app: an exported bundle inlines this whole build into one
// page and must not drag the app's bundle along with it.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    outDir: path.resolve(__dirname, '../pkg/gui/dist/player'),
    emptyOutDir: true,
    // A bundle inlines this build into a page opened from file://, where a module
    // script is refused by CORS and dynamic imports cannot resolve. One classic
    // script and one stylesheet is what can be inlined.
    cssCodeSplit: false,
    rollupOptions: {
      input: { player: path.resolve(__dirname, 'player.html') },
      output: { format: 'iife', inlineDynamicImports: true }
    }
  }
});
