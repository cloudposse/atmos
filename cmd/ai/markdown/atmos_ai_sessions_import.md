Import an AI chat session from a checkpoint file.

Restores a session from a checkpoint file created with 'atmos ai sessions export'.
The imported session can be resumed with 'atmos ai chat --session <name>'.

Supports JSON and YAML checkpoint files.

Examples:
  atmos ai sessions import session.json
  atmos ai sessions import backup.yaml --name restored-session
  atmos ai sessions import session.json --overwrite
