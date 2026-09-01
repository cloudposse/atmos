import React from 'react';
import Layout from '@theme/Layout';
import useGlobalData from '@docusaurus/useGlobalData';
import ScorecardTable from '@site/src/components/SecurityPosture/ScorecardTable';
import BestPracticesBadge from '@site/src/components/SecurityPosture/BestPracticesBadge';
import styles from './security.module.css';

const SCORECARD_VIEWER_URL = 'https://scorecard.dev/viewer/?uri=github.com/cloudposse/atmos';
const BEST_PRACTICES_PROJECT_ID = 14393;
const BEST_PRACTICES_URL = `https://www.bestpractices.dev/projects/${BEST_PRACTICES_PROJECT_ID}`;

interface ScorecardData {
  date: string;
  score: number;
  checks: Array<{
    name: string;
    score: number;
    reason: string;
    documentation?: { url?: string; short?: string };
  }>;
}

interface SecurityPostureData {
  scorecard: ScorecardData | null;
  bestPractices: {
    name: string;
    repo_url: string;
    badge_level: string;
    achieve_passing_status?: string;
    updated_at?: string;
  } | null;
  fetchedAt: string;
}

export default function SecurityPage() {
  const globalData = useGlobalData();
  const data: SecurityPostureData | undefined =
    globalData['fetch-security-posture']?.default;

  const scorecard = data?.scorecard ?? null;
  const bestPractices = data?.bestPractices ?? null;
  const fetchedAt = data?.fetchedAt
    ? new Date(data.fetchedAt).toLocaleDateString('en-US', {
        year: 'numeric',
        month: 'long',
        day: 'numeric',
      })
    : null;

  return (
    <Layout
      title="Security & Trust"
      description="Atmos publishes its OpenSSF Scorecard and Best Practices badge results with live links to the authoritative source, so enterprises evaluating Atmos can verify our security posture themselves."
    >
      <main className={styles.security}>
        <section className={styles.hero}>
          <p className={styles.eyebrow}>Security</p>
          <h1>Security &amp; trust</h1>
          <p className={styles.lede}>
            We publish our OpenSSF security posture with live data and direct
            links to the source, so teams evaluating Atmos can verify it
            themselves instead of trusting a static badge.
          </p>
        </section>

        <section className={styles.verifyPanel} aria-labelledby="verify-yourself">
          <h2 id="verify-yourself">Verify this yourself</h2>
          <p>
            The scores below are pulled directly from the same APIs that power
            these public tools. Don&apos;t take our word for it — check the
            source.
          </p>
          <div className={styles.verifyLinks}>
            <a
              className="button button--primary"
              href={SCORECARD_VIEWER_URL}
              target="_blank"
              rel="noreferrer"
            >
              OpenSSF Scorecard viewer
            </a>
            <a
              className="button button--secondary"
              href={BEST_PRACTICES_URL}
              target="_blank"
              rel="noreferrer"
            >
              Best Practices project page
            </a>
          </div>
        </section>

        <section className={styles.summaryGrid}>
          <BestPracticesBadge bestPractices={bestPractices} projectId={BEST_PRACTICES_PROJECT_ID} />

          <div className={styles.panel}>
            <h2>OpenSSF Scorecard</h2>
            {scorecard ? (
              <>
                <div className={styles.scoreRow}>
                  <span className={styles.scoreValue}>{scorecard.score.toFixed(1)}</span>
                  <span className={styles.scoreOutOf}>/ 10</span>
                </div>
                <p>
                  Across {scorecard.checks.length} automated checks. Verify at{' '}
                  <a href={SCORECARD_VIEWER_URL} target="_blank" rel="noreferrer">
                    scorecard.dev
                  </a>
                  .
                </p>
              </>
            ) : (
              <p>
                Data temporarily unavailable — verify directly at{' '}
                <a href={SCORECARD_VIEWER_URL} target="_blank" rel="noreferrer">
                  scorecard.dev
                </a>
                .
              </p>
            )}
          </div>
        </section>

        {fetchedAt && <p className={styles.fetchedAt}>Data fetched as of {fetchedAt}.</p>}

        {scorecard && (
          <section className={styles.checksSection} aria-labelledby="scorecard-checks">
            <h2 id="scorecard-checks">Scorecard checks</h2>
            <ScorecardTable checks={scorecard.checks} />
          </section>
        )}

        <section className={styles.hardeningNote} aria-labelledby="hardening">
          <h2 id="hardening">Actively hardening</h2>
          <p>
            Security posture is an ongoing effort, not a one-time score. We
            track open gaps against these checks and work through them as part
            of our normal development process, the same way we track any other
            open issue.
          </p>
        </section>
      </main>
    </Layout>
  );
}
