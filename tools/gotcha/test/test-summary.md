# Test Results

[![Passed](https://shields.io/badge/PASSED-1-success?style=for-the-badge)](#user-content-passed) [![Failed](https://shields.io/badge/FAILED-1-critical?style=for-the-badge)](#user-content-failed) [![Skipped](https://shields.io/badge/SKIPPED-0-inactive?style=for-the-badge)](#user-content-skipped) 

### ❌ Failed Tests (1)

<details>
<summary>Click to see failed tests</summary>

| Test | Package | Duration |
|------|---------|----------|
| `TestFail` | pkg | 1.00s |

**Run locally to reproduce:**
```bash
go test test/pkg -run ^TestFail$ -v
```
</details>

### ✅ Passed Tests (1)

<details>
<summary>Click to show all passing tests</summary>

| Test | Package | Duration | % of Total |
|------|---------|----------|----------|
| `TestPass1` | pkg | 0.50s | 100.0% |


</details>

# Test Coverage

| Metric | Coverage | Details |
|--------|----------|----------|
| Statement Coverage | 75.0% | 🟡 |

