# Filter Semgrep SARIF

Semgrep includes inline-suppressed findings in SARIF with `inSource` suppression
metadata. [GitHub code scanning does not consume that metadata](https://docs.github.com/en/code-security/reference/code-scanning/sarif-files/sarif-support),
so this action removes those findings before upload. All unsuppressed findings
and report metadata remain intact. Rejected or pending suppressions remain findings.

```yaml
- uses: $/.github/actions/filter-semgrep-sarif
  with:
    input-file: semgrep.sarif
    output-file: semgrep-upload.sarif
```

Both file paths are required and resolve relative to the working directory.
Use different paths to preserve the original report. The action uses
`actions/github-script`, makes no API calls, and needs no secrets or elevated
permissions. Only upload the output after this action succeeds.

Run the regression tests with:

```sh
node --test .github/actions/filter-semgrep-sarif/filter-semgrep-sarif.test.cjs
```

`atmos test local-actions javascript` and the local action CI workflow discover
the tests automatically. They verify that real unpinned-action findings survive,
suppression statuses are respected, metadata is preserved, and malformed reports fail.
