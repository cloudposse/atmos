# Test every supported component in a stack
atmos stack test --stack=dev

# Test selected components; repeat the flag or comma-separate names
atmos stack test --stack=dev --component=network,app --component=api

# Pick components interactively and preview their test commands
atmos stack test --stack=dev --component --dry-run

# Test affected components and their prerequisites
atmos stack test --affected --base=origin/main --include-dependencies
