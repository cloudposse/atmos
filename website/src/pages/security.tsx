import React, { useEffect, useState } from 'react';
import Layout from '@theme/Layout';
import useGlobalData from '@docusaurus/useGlobalData';
import { RiShieldCheckLine } from 'react-icons/ri';
import ScorecardTable from '@site/src/components/SecurityPosture/ScorecardTable';
import BestPracticesBadge from '@site/src/components/SecurityPosture/BestPracticesBadge';
import ScoreGauge from '@site/src/components/SecurityPosture/ScoreGauge';
import styles from './security.module.css';

const SCORECARD_VIEWER_URL = 'https://scorecard.dev/viewer/?uri=github.com/cloudposse/atmos';
const BEST_PRACTICES_PROJECT_ID = 14393;
const BEST_PRACTICES_URL = `https://www.bestpractices.dev/projects/${BEST_PRACTICES_PROJECT_ID}`;
const SECURITY_POLICY_URL = 'https://github.com/cloudposse/atmos/security/policy';
const SECURITY_ADVISORIES_URL = 'https://github.com/cloudposse/atmos/security/advisories';

interface ScorecardData {
  date: string;
  score: number;
  repo?: { name?: string; commit?: string };
  scorecard?: { version?: string };
  checks: Array<{
    name: string;
    score: number;
    reason: string;
    details?: string[] | null;
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

function formatDate(iso: string, timeZone?: string): string {
  const date = new Date(iso);
  const zoneOpt = timeZone ? { timeZone } : {};
  // dateStyle/timeStyle can't be combined with timeZoneName (throws), so format the
  // zone abbreviation separately and append it.
  const formatted = new Intl.DateTimeFormat('en-US', {
    dateStyle: 'long',
    timeStyle: 'short',
    ...zoneOpt,
  }).format(date);
  const zoneName = new Intl.DateTimeFormat('en-US', {
    timeZoneName: 'short',
    ...zoneOpt,
  })
    .formatToParts(date)
    .find((part) => part.type === 'timeZoneName')?.value;

  return zoneName ? `${formatted} ${zoneName}` : formatted;
}

// Server-rendered HTML always uses UTC (the build machine's zone shouldn't leak into
// static output); after hydration we re-render in the visitor's local zone. Doing the
// swap in an effect (post-hydration) avoids a React hydration mismatch.
function useLocalDate(iso: string | null): string | null {
  const [formatted, setFormatted] = useState<string | null>(() =>
    iso ? formatDate(iso, 'UTC') : null,
  );

  useEffect(() => {
    if (iso) setFormatted(formatDate(iso));
  }, [iso]);

  return formatted;
}

export default function SecurityPage() {
  const globalData = useGlobalData();
  const data: SecurityPostureData | undefined =
    globalData['fetch-security-posture']?.default;

  const scorecard = data?.scorecard ?? null;
  const bestPractices = data?.bestPractices ?? null;
  const fetchedAt = useLocalDate(data?.fetchedAt ?? null);
  const scanDate = useLocalDate(scorecard?.date ?? null);
  const shortCommit = scorecard?.repo?.commit ? scorecard.repo.commit.slice(0, 7) : null;

  return (
    <Layout
      title="Security & Trust"
      description="Atmos publishes its OpenSSF Scorecard and Best Practices badge results with live links to the authoritative source, so enterprises evaluating Atmos can verify our security posture themselves."
    >
      <main className={styles.security}>
        <section className={styles.hero}>
          <p className={styles.eyebrow}>Security</p>
          <h1>Security &amp; Trust</h1>
          <p className={styles.lede}>
            We publish our OpenSSF security posture from a build-time snapshot
            and direct links to the source, so teams evaluating Atmos can
            verify it themselves instead of trusting a static badge.
          </p>
        </section>

        <section className={styles.aboutSection} aria-labelledby="about-openssf">
          <h2 id="about-openssf">What is OpenSSF?</h2>
          <p>
            The{' '}
            <a href="https://openssf.org" target="_blank" rel="noreferrer">
              Open Source Security Foundation
            </a>{' '}
            (OpenSSF) is a Linux Foundation project that runs two independent,
            automated assessments of open source projects: Scorecard, which
            checks a repository against ~18 supply-chain security practices,
            and the Best Practices badge, which verifies a project against a
            broader set of security and quality criteria. Both re-scan and
            re-verify Atmos on an ongoing basis — these aren&apos;t one-time
            certifications, they&apos;re a continuously updated assessment.
            Verify it yourself at the{' '}
            <a href={SCORECARD_VIEWER_URL} target="_blank" rel="noreferrer">
              Scorecard viewer
            </a>{' '}
            or the{' '}
            <a href={BEST_PRACTICES_URL} target="_blank" rel="noreferrer">
              Best Practices project page
            </a>
            .
          </p>
        </section>

        <section className={styles.reportCard} aria-labelledby="scorecard-heading">
          <h2 id="scorecard-heading" className={styles.reportHeading}>
            <RiShieldCheckLine className={styles.reportIcon} />
            OpenSSF Scorecard Report
          </h2>

          <BestPracticesBadge bestPractices={bestPractices} projectId={BEST_PRACTICES_PROJECT_ID} />

          {scorecard ? (
            <>
              <div className={styles.summaryRow}>
                <ScoreGauge score={scorecard.score} />
                <dl className={styles.meta}>
                  <div>
                    <dt>Repository</dt>
                    <dd>{scorecard.repo?.name ?? 'github.com/cloudposse/atmos'}</dd>
                  </div>
                  {shortCommit && (
                    <div>
                      <dt>Commit</dt>
                      <dd>{shortCommit}</dd>
                    </div>
                  )}
                  {scorecard.scorecard?.version && (
                    <div>
                      <dt>Scorecard version</dt>
                      <dd>{scorecard.scorecard.version}</dd>
                    </div>
                  )}
                  {scanDate && (
                    <div>
                      <dt>Scan generated</dt>
                      <dd>{scanDate}</dd>
                    </div>
                  )}
                </dl>
              </div>

              <ScorecardTable checks={scorecard.checks} />
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

          {fetchedAt && (
            <p className={styles.fetchedAt}>Data fetched at build time: {fetchedAt}.</p>
          )}
        </section>

        <section className={styles.aboutSection} aria-labelledby="hardening">
          <h2 id="hardening">Actively hardening</h2>
          <p>
            Security posture is an ongoing effort, not a one-time score. We
            track open gaps against these checks and work through them as part
            of our normal development process, the same way we track any other
            open issue.
          </p>
          <p>
            Found a vulnerability? See our{' '}
            <a href={SECURITY_POLICY_URL} target="_blank" rel="noreferrer">
              security policy
            </a>{' '}
            for how to report it. Disclosed issues are published as{' '}
            <a href={SECURITY_ADVISORIES_URL} target="_blank" rel="noreferrer">
              security advisories
            </a>
            .
          </p>
        </section>
      </main>
    </Layout>
  );
}
