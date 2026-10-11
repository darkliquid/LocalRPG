// Mirrors the roll card's authority: a narration sentence must not carry the dice,
// because the card already shows the notation and the total. This is deliberately
// conservative. It removes a fragment only when it is unambiguously a roll report,
// and it leaves a number in ordinary prose alone.
const DICE = /(?:\d*)d\d+(?:[+-]\d+)?/i;
const ARITHMETIC = /\d+\s*(?:[+-]\s*\d+\s*)*=\s*\d+/;
const REPORT_PREFIX = /^(?:roll|rolled|total|totals|dice|result)\b/i;

function isTelegraphy(fragment: string): boolean {
  return DICE.test(fragment) || ARITHMETIC.test(fragment);
}

// isRollReportLine reports whether a whole line is nothing but a roll report, so it
// can be dropped rather than leaving an empty shelf where the dice were.
function isRollReportLine(line: string): boolean {
  if (!isTelegraphy(line)) return false;
  if (REPORT_PREFIX.test(line)) return true;
  const residual = line
    .replace(new RegExp(DICE.source, 'gi'), '')
    .replace(new RegExp(ARITHMETIC.source, 'g'), '')
    .replace(/[^a-z]+/gi, '');
  return residual === '';
}

function removeInlineFragments(line: string): string {
  return line
    .replace(/\([^)]*\)|\[[^\]]*\]/g, (match) => (isTelegraphy(match) ? '' : match))
    .replace(/[ \t]{2,}/g, ' ')
    .replace(/[ \t]+([.,;:!?])/g, '$1')
    .replace(/[ \t]+$/, '');
}

// stripDiceTelegraphy removes dice arithmetic from a narration string, so the roll
// card is the only place a number appears. It is not applied to speech: a character
// saying "a seven!" is dialogue, not a roll report.
export function stripDiceTelegraphy(text: string): string {
  if (!text) return text;
  const kept: string[] = [];
  for (const line of text.split('\n')) {
    if (line.trim() !== '' && isRollReportLine(line.trim())) continue;
    kept.push(removeInlineFragments(line));
  }
  return kept.join('\n').replace(/^\n+|\n+$/g, '');
}