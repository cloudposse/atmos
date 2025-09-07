---
name: analyze-pr
description: Analyze a PR for review feedback and failing checks
---

Please analyze PR #{{PR_NUMBER}} in the cloudposse/atmos repository:

1. Fetch all review comments (CodeRabbit and human)
2. Check status of all CI/CD checks
3. Identify failing tests or security scans
4. Create a remediation plan
5. Present the plan for my approval

Use the pr-review-remediator agent if available (see ../agents/pr-review-remediator.md).

Focus on:
- Validating CodeRabbit suggestions make sense
- Only linting changed files
- Following Atmos coding standards from CLAUDE.md
- Providing clear reasoning for each suggestion