import '@testing-library/jest-dom/vitest';
import { afterEach } from 'vitest';
import { cleanup } from '@testing-library/react';

// jsdom implements neither of these, and the drawers and the tree construct one.
// The guard keeps the setup usable in a node-environment test, which has no window.
if (typeof window !== 'undefined') {
  class ResizeObserverStub {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  globalThis.ResizeObserver = ResizeObserverStub as unknown as typeof ResizeObserver;

  if (!window.matchMedia) {
    window.matchMedia = ((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener() {},
      removeListener() {},
      addEventListener() {},
      removeEventListener() {},
      dispatchEvent() {
        return false;
      },
    })) as unknown as typeof window.matchMedia;
  }
}

// Tests import describe/it/expect explicitly, so RTL's automatic cleanup does not
// run; without this a second render in a file sees the first one's DOM. A
// node-environment test has no DOM to clean up.
if (typeof window !== 'undefined') {
  afterEach(() => {
    cleanup();
  });
}
