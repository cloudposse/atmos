```bash
# Verify the workflow file exists
ls -la stacks/workflows/

# Check your atmos.yaml for workflow paths configuration
cat atmos.yaml | grep -A5 workflows

# List workflows from existing files
atmos describe workflows
```
