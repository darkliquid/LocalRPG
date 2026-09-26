import { useEffect, useState } from 'react';

export type TransitionState = 'enter' | 'exit';

interface MountTransitionResult {
  mounted: boolean;
  state: TransitionState;
}

export function useMountTransition(isOpen: boolean, duration = 250): MountTransitionResult {
  const [mounted, setMounted] = useState(isOpen);
  const [state, setState] = useState<TransitionState>(isOpen ? 'enter' : 'exit');

  useEffect(() => {
    if (isOpen) {
      // Enter synchronously. Deferring it (e.g. to a rAF) renders one frame with
      // the exit class, so the drawer flashes in at its resting position and then
      // jumps to the start of the enter animation.
      setMounted(true);
      setState('enter');
      return;
    }

    if (!mounted) {
      return;
    }

    setState('exit');
    const timer = window.setTimeout(() => setMounted(false), duration);
    return () => window.clearTimeout(timer);
  }, [isOpen, mounted, duration]);

  return { mounted, state };
}
