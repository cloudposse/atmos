# SARIF result.stacks has uniqueItems: true. Preserve the first occurrence of
# each complete stack object, including its frame order and metadata.
.runs[].results[]? |= (
  if has("stacks") then
    .stacks |= reduce .[] as $stack ([];
      if index($stack) == null then . + [$stack] else . end)
  else . end
)
