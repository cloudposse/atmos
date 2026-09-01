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
