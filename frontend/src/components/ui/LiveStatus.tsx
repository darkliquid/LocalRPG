export interface LiveStatusProps {
  /** message is the sentence to announce; empty announces nothing. */
  message: string;
}

// LiveStatus is a live region that is always in the tree and empty until there is
// something to say. A region inserted at the same moment as its text is announced
// by few screen readers, so the element is rendered unconditionally and only its
// content changes.
export function LiveStatus({ message }: LiveStatusProps) {
  return (
    <p role="status" className="sr-only">
      {message}
    </p>
  );
}
