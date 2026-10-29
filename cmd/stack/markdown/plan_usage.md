# Plan all supported components in a stack.
atmos stack plan --stack=prod

# Select several components with repeated or comma-separated names.
atmos stack plan --stack=prod --component=network,app --component=database

# Choose components and preview commands without executing plans or hooks.
atmos stack plan --stack=prod --component --dry-run

# Plan affected components and their prerequisites.
atmos stack plan --affected --base=origin/main --include-dependencies
