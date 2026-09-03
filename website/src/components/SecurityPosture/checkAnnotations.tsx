import React, { type ReactNode } from 'react';

export interface CheckAnnotation {
  // Why the raw score doesn't reflect our actual risk, given our real release process
  // and repo configuration. Keep this current if the underlying process changes.
  note: ReactNode;
}

const MERGE_QUEUE_DOCS_URL =
  'https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/configuring-pull-request-merges/managing-a-merge-queue';

export const CHECK_ANNOTATIONS: Record<string, CheckAnnotation> = {
  'Branch-Protection': {
    note: (
      <>
        We use a{' '}
        <a href={MERGE_QUEUE_DOCS_URL} target="_blank" rel="noreferrer">
          merge queue
        </a>{' '}
        specifically so branches don&apos;t need to be up to date before merging — the
        queue re-validates every change against the latest main at merge time, which
        makes requiring &lsquo;up to date branches&rsquo; on main redundant. Feature
        releases are cut from unprotected feature branches, but production releases are
        different: they build from a separate GitHub environment, are marked as
        test/pre-release, and can only be cut from main through environment protection
        rules. An unprotected feature branch doesn&apos;t expose production releases.
      </>
    ),
  },
  Vulnerabilities: {
    note:
      'Scorecard flags advisories by module version, not by which code paths we ' +
      "actually call, so some of these can't be cleared by upgrading. " +
      'golang.org/x/crypto/openpgp is a permanent upstream deprecation notice with no ' +
      "fixed version — we depend on golang.org/x/crypto for nacl/box, never openpgp. " +
      'The aws-sdk-go v1 s3crypto advisories come from gomplate’s AWS datasources, ' +
      "transitively via HashiCorp Vault's AWS auth backend, which still ships against " +
      'SDK v1; there is no upstream release that removes it today. We track and pick up ' +
      'real fixes as they ship, like the golang.org/x/crypto SSH bump that closed the ' +
      'other advisories in this same check.',
  },
};
