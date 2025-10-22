# Atmos Scripts

This directory contains utility scripts for maintaining the Atmos project.

## generate-notice.sh

Generates the `NOTICE` file containing license attributions for all Go dependencies that require it.

### Usage

```bash
./scripts/generate-notice.sh
```

### What It Does

1. Uses `go-licenses` to scan all Go dependencies
2. Generates a comprehensive NOTICE file with:
   - Apache-2.0 licensed dependencies (123)
   - BSD licensed dependencies (66)
   - MPL-2.0 licensed dependencies (29)
   - MIT licensed dependencies (163)
3. Includes URLs to each dependency's license file

### Output

Creates or updates `NOTICE` in the repository root with:
- Copyright header for Atmos
- Categorized dependency lists by license type
- Direct links to license files in upstream repositories
- Instructions for viewing full license texts

### When to Run

Run this script:
- After adding new Go dependencies
- Before major releases
- To update copyright year
- Periodically to keep NOTICE file current

### Requirements

- `go-licenses` tool (will auto-install if missing)
- Internet connection (to generate license report)
- Write access to repository root

### Example Output

```
Generating NOTICE file for Atmos...
Working directory: /path/to/atmos
Generating license report...
Found 405 total dependencies
  - 123 Apache-2.0 licenses
  - 66 BSD licenses
✅ NOTICE file generated successfully: /path/to/atmos/NOTICE

Summary:
  - Total dependencies: 405
  - Apache-2.0: 123
  - BSD: 66
  - MPL-2.0: 29
  - MIT: 163

Review the NOTICE file and commit it to the repository.
```

### Integration with CI/CD

License compliance is enforced by GitHub's Dependency Review action (`.github/workflows/dependency-review.yml`) which:
- Validates licenses on all pull requests
- Blocks dependencies with copyleft or restrictive licenses
- Checks for security vulnerabilities
- Uses GitHub's native license detection (no custom tooling required)

The NOTICE file is **manually generated** and committed to the repository. This is intentional to:
- Allow human review of license changes
- Keep the file stable across builds
- Avoid CI/CD flakiness from external tools
- Separate compliance validation (automated) from documentation (manual)

### Customization

Edit the script to:
- Change the copyright header
- Modify license groupings
- Add/remove license categories
- Customize the footer message

### Troubleshooting

**"go-licenses: command not found"**
- The script will auto-install it via `go install`

**"Failed to find license for X"**
- Some packages have non-standard license file names
- Check the package manually and add to ignore list if needed
- See `go-licenses` documentation for `--ignore` flag

**"One or more libraries have incompatible/unknown license"**
- This warning can be ignored for known cases (xi2/xz is public domain)
- Review unknown licenses manually before proceeding

### Related Files

- `.github/workflows/dependency-review.yml` - GitHub's native license validation workflow
- `NOTICE` - Generated attribution file (do not edit manually, regenerate with this script)
