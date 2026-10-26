export function stripFrontmatter(content: string): string {
  if (!content.startsWith('---')) return content;

  const lines = content.split(/\r?\n/);
  if (lines[0].trim() !== '---') return content;

  const endIndex = lines.findIndex((line, index) => index > 0 && line.trim() === '---');
  if (endIndex === -1) return content;

  return lines.slice(endIndex + 1).join('\n').trim();
}
