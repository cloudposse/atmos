/**
 * Derive `<permalink>.md` from a doc's permalink, normalizing the root and any
 * trailing slash:
 *   ''                     → ''
 *   /                      → /index.md
 *   /cli/                  → /cli.md
 *   /cli/commands/version  → /cli/commands/version.md
 *
 * Shared between the DocBreadcrumbs and DocItem/Content swizzles so the
 * `<link rel="alternate" type="text/markdown">` in <head> and the "Copy
 * Markdown" button in the breadcrumb row always point at the same URL.
 */
export function deriveMarkdownHref(permalink: string): string {
  if (!permalink) return '';
  // Linear trailing-slash trim (CodeQL-safe; avoids /\/+$/).
  let end = permalink.length;
  while (end > 0 && permalink.charCodeAt(end - 1) === 47) end -= 1; // '/'
  const normalized = permalink.slice(0, end);
  return normalized ? normalized + '.md' : '/index.md';
}
