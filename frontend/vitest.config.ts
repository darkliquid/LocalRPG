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
    coverage: {
      provider: 'v8',
      // src/ rather than the default, which counts only the files a test happened to
      // import and so flatters the number by leaving the rest out of it entirely.
      include: ['src/**'],
      reporter: ['text-summary', 'json-summary'],
      // Floors, not targets. They sit just below what the suite reaches today, so
      // they can only rise. The suite runs in jsdom over a fixed set of files, so
      // the numbers are stable: a drop means a test was lost or a branch went
      // untested. Raise them deliberately, in a commit that says why.
      thresholds: {
        statements: 33,
        lines: 34,
        functions: 33,
        branches: 30,
      },
    },
  },
});
