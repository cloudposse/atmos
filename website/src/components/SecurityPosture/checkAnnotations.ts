export interface CheckAnnotation {
  // Why the raw score doesn't reflect our actual risk, given our real release process
  // and repo configuration. Keep this current if the underlying process changes.
  note: string;
}

export const CHECK_ANNOTATIONS: Record<string, CheckAnnotation> = {
  'Branch-Protection': {
    note:
      "We use a merge queue specifically so branches don't need to be up to date before " +
      "merging — the queue re-validates every change against the latest main at merge " +
      "time, which makes requiring ‘up to date branches’ on main redundant. " +
      'Feature releases are cut from unprotected feature branches, but production ' +
      'releases are different: they build from a separate GitHub environment, are ' +
      'marked as test/pre-release, and can only be cut from main through environment ' +
      "protection rules. An unprotected feature branch doesn't expose production " +
      'releases.',
  },
};
