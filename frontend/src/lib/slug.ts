// Mirrors entity.Slugify in the Go backend: lowercase, [a-z0-9] kept, runs of
// spaces/hyphens/underscores collapse to a single hyphen, trailing hyphen trimmed.
export const slugify = (name: string): string => {
  let out = '';
  let precededBySeparator = true;
  for (const ch of name.toLowerCase().trim()) {
    if ((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')) {
      out += ch;
      precededBySeparator = false;
    } else if (ch === ' ' || ch === '-' || ch === '_') {
      if (out.length > 0 && !precededBySeparator) {
        out += '-';
        precededBySeparator = true;
      }
    }
  }
  return out.endsWith('-') ? out.slice(0, -1) : out;
};
