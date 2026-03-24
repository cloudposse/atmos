Execute an AI prompt non-interactively and output the result.

This command is designed for automation, scripting, and CI/CD integration.
It executes a single prompt and outputs the result without any interactive UI.

The prompt can be provided as:
- Command arguments: atmos ai exec "your prompt here"
- Stdin (pipe): echo "your prompt" | atmos ai exec

Output can be formatted as:
- text (default): Plain text response
- json: Structured JSON with metadata
- markdown: Formatted markdown

Exit codes:
- 0: Success
- 1: AI error (API failure, invalid response)
- 2: Tool execution error

Examples:
  # Simple question
  atmos ai exec "List all available stacks"

  # With JSON output
  atmos ai exec "Describe the vpc component" --format json

  # Save output to file
  atmos ai exec "Analyze prod stack" --output analysis.json --format json

  # Disable tool execution
  atmos ai exec "Explain Atmos concepts" --no-tools

  # Pipe prompt from stdin
  echo "Validate stack configuration" | atmos ai exec --format json

  # Use in CI/CD pipeline
  result=$(atmos ai exec "Check for security issues" --format json)
  if echo "$result" | jq -e '.success == false'; then
    exit 1
  fi
