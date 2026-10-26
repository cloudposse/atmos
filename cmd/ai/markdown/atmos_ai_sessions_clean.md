Remove old AI chat sessions based on retention policy.

Sessions older than the specified duration will be permanently deleted.
Use this command to free up space and remove outdated conversations.

Examples:
  atmos ai sessions clean --older-than 30d   # Delete sessions older than 30 days
  atmos ai sessions clean --older-than 7d    # Delete sessions older than 7 days
  atmos ai sessions clean --older-than 24h   # Delete sessions older than 24 hours
