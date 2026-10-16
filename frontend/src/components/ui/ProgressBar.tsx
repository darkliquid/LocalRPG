export interface ProgressBarProps {
  value: number;
  /** max is the value the bar reports as full; it defaults to 100. */
  max?: number;
  /**
   * label names the bar for assistive technology, so it is required rather than
   * optional decoration: a progressbar with no name is a number with no meaning.
   */
  label: string;
  /** className styles the track, which every caller draws differently. */
  className?: string;
  /** barClassName styles the fill. */
  barClassName?: string;
}

// ProgressBar is the one way a drawn bar states its value. The ratio is computed
// once, so the fill's width and the announced number are derived from the same
// figure and cannot disagree; a maximum of zero is an empty bar rather than a
// division by zero.
export function ProgressBar({ value, max = 100, label, className, barClassName }: ProgressBarProps) {
  const ratio = max > 0 ? Math.min(Math.max(value, 0), max) / max : 0;

  return (
    <div className={className}>
      <div
        role="progressbar"
        aria-valuenow={ratio * max}
        aria-valuemin={0}
        aria-valuemax={max > 0 ? max : 0}
        aria-label={label}
        className={barClassName}
        style={{ width: `${ratio * 100}%` }}
      />
    </div>
  );
}
