Export an AI chat session to a checkpoint file for backup or sharing.

The checkpoint file contains the complete session including:
- Session metadata (name, model, provider, timestamps)
- Complete message history
- Project context (optional)
- Statistics

Supports multiple formats: JSON (default), YAML, Markdown

Examples:
  atmos ai sessions export vpc-migration --output session.json
  atmos ai sessions export my-session --output backup.yaml --context
  atmos ai sessions export analysis --output report.md --format markdown
