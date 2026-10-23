// GitHub code scanning does not honor SARIF suppressions. Remove only findings
// Semgrep marked as suppressed in source before uploading; retain all others.
module.exports = function filterSemgrepSarif(report) {
  for (const run of report.runs) {
    if (!run.results) continue;
    run.results = run.results.filter(result => !result.suppressions?.some(
      suppression => suppression.kind === 'inSource'
        && (suppression.status === undefined || suppression.status === 'accepted')
    ));
  }
  return report;
};
