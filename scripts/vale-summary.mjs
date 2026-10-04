// Summarise a Vale JSON report: totals, the noisiest rules, and the worst files.
//
// This exists because the configured styles currently find tens of thousands of
// alerts, so a full report buries a CI log. The counts are what tell you whether
// the situation is getting better or worse.
//
// Usage: node scripts/vale-summary.mjs <report.json> <scoped-file-count>
import { readFileSync } from 'node:fs';

const [, , reportPath, scopedArg] = process.argv;
const results = JSON.parse(readFileSync(reportPath, 'utf8'));

const severity = {};
const rules = {};
const perFile = {};

for (const [file, alerts] of Object.entries(results)) {
  if (alerts.length) perFile[file] = alerts.length;
  for (const alert of alerts) {
    severity[alert.Severity] = (severity[alert.Severity] ?? 0) + 1;
    const key = `${alert.Severity}  ${alert.Check}`;
    rules[key] = (rules[key] ?? 0) + 1;
  }
}

const total = Object.values(severity).reduce((sum, count) => sum + count, 0);
// Vale only lists files that have findings, so the scoped count is passed in
// rather than read from the result keys.
const scoped = Number(scopedArg);
const errors = severity.error ?? 0;

console.log(`Vale: ${total} alerts in ${Object.keys(perFile).length} of ${scoped} files`);
console.log(
  `  errors ${errors}, warnings ${severity.warning ?? 0}, suggestions ${severity.suggestion ?? 0}`,
);

console.log('\nNoisiest rules:');
for (const [key, count] of Object.entries(rules).sort((a, b) => b[1] - a[1]).slice(0, 10)) {
  console.log(`  ${String(count).padStart(6)}  ${key}`);
}

console.log('\nWorst files:');
for (const [file, count] of Object.entries(perFile).sort((a, b) => b[1] - a[1]).slice(0, 5)) {
  console.log(`  ${String(count).padStart(6)}  ${file}`);
}

// The error count becomes the exit code so the shell can decide whether to gate,
// keeping summary mode and full mode in agreement.
process.exit(errors > 255 ? 255 : errors);
