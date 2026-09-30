import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import path from 'path';

// The player is built on its own, into its own directory, so it shares no chunks
// or stylesheets with the app: an exported bundle copies this whole directory and
// must not drag the app's bundle along with it.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  // Relative asset paths, because a bundle is opened from a file rather than served
  // from a web root.
  base: './',
  build: {
    outDir: path.resolve(__dirname, '../pkg/gui/dist/player'),
    emptyOutDir: true,
    rollupOptions: {
      input: { player: path.resolve(__dirname, 'player.html') }
    }
  }
});
