export type RiskLevel = 'Critical' | 'High' | 'Medium' | 'Low';

// Source: https://github.com/ossf/scorecard/blob/main/docs/checks.md
// The Scorecard API doesn't return a risk level per check, so this mirrors OpenSSF's
// own published classification. Spot-check against upstream if Scorecard adds checks.
export const CHECK_RISK_LEVEL: Record<string, RiskLevel> = {
  'Binary-Artifacts': 'High',
  'Branch-Protection': 'High',
  'CI-Tests': 'Low',
  'CII-Best-Practices': 'Low',
  'Code-Review': 'High',
  Contributors: 'Low',
  'Dangerous-Workflow': 'Critical',
  'Dependency-Update-Tool': 'High',
  Fuzzing: 'Medium',
  License: 'Low',
  Maintained: 'High',
  Packaging: 'Medium',
  'Pinned-Dependencies': 'Medium',
  SAST: 'Medium',
  SBOM: 'Medium',
  'Security-Policy': 'Medium',
  'Signed-Releases': 'High',
  'Token-Permissions': 'High',
  Vulnerabilities: 'High',
  Webhooks: 'Critical',
};

const RISK_ORDER: Record<RiskLevel, number> = { Critical: 0, High: 1, Medium: 2, Low: 3 };

// The Scorecard API returns checks in whatever order it happened to run them in, which
// reads as arbitrary to a viewer. Surface the checks that matter most first: highest
// risk level, then (within a risk level) worst score first.
export function sortChecksByRisk<T extends { name: string; score: number }>(checks: T[]): T[] {
  return [...checks].sort((a, b) => {
    const riskA = RISK_ORDER[CHECK_RISK_LEVEL[a.name]] ?? RISK_ORDER.Low + 1;
    const riskB = RISK_ORDER[CHECK_RISK_LEVEL[b.name]] ?? RISK_ORDER.Low + 1;
    if (riskA !== riskB) return riskA - riskB;
    return a.score - b.score;
  });
}
