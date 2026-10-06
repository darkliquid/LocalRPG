import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

// The app's own vite.config.ts sets an outDir inside pkg/gui and loads the
// Tailwind plugin; neither belongs in a unit run, so the test config is separate
// and loads only what a jsdom render needs.
export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    include: ['src/**/*.test.{ts,tsx}'],
    css: false,
  },
});
