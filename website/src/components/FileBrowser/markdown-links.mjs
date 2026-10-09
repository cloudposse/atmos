/** Resolve README links from the source file, regardless of the page's trailing slash. */
export function resolveMarkdownLink(href, filePath, routeBasePath, githubUrl) {
  if (!href || /^(?:[a-z][a-z\d+.-]*:|\/|#|\?)/i.test(href)) return href;
  const base = `/${routeBasePath.replace(/^\/+|\/+$/g, '')}`;
  const source = filePath.split('/').map(encodeURIComponent).join('/');
  const resolved = new URL(href, `https://file-browser.invalid${base}/${source}`);
  if (githubUrl && resolved.pathname !== base && !resolved.pathname.startsWith(`${base}/`)) {
    return new URL(href, githubUrl).href;
  }
  return `${resolved.pathname}${resolved.search}${resolved.hash}`;
}
