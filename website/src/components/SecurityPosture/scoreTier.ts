export type ScoreTier = 'good' | 'warn' | 'bad' | 'neutral';

// OpenSSF Scorecard uses -1 as a sentinel for "inconclusive" (not a failing score).
export function scoreTier(score: number): ScoreTier {
  if (score < 0) return 'neutral';
  if (score >= 8) return 'good';
  if (score >= 4) return 'warn';
  return 'bad';
}
