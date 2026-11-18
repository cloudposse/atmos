import { useEffect } from 'react';
import { useShadowRootElements } from '@backstage/plugin-techdocs-react';

// TechDocs sanitizes arbitrary MkDocs stylesheet links. Use its addon API to
// install typography inside the reader's shadow root on every page navigation.
const styles = "/* TechDocs renders MkDocs in a shadow root; keep prose readable independently\n   of the compact dashboard type scale and MkDocs' responsive rem scaling. */\n.md-typeset {\n  font-family: 'Geist Variable', -apple-system, BlinkMacSystemFont, sans-serif;\n  font-size: 16px !important;\n  line-height: 1.65;\n}\n.md-typeset h1 { font-size: 28px !important; line-height: 1.25 !important; }\n.md-typeset h2 { font-size: 22px !important; line-height: 1.35 !important; }\n.md-typeset h3 { font-size: 18px !important; line-height: 1.4 !important; }\n.md-typeset code { font-family: 'Geist Mono Variable', ui-monospace, monospace; font-size: 0.875em; }\n.md-typeset table:not([class]) { font-size: 14px !important; }\n.md-typeset table:not([class]) td,\n.md-typeset table:not([class]) th { padding: 12px 16px; }\n.md-main__inner { margin-top: 24px !important; }\n@media (max-width: 600px) {\n  .md-content__inner { margin-left: 0; margin-right: 0; }\n  .md-typeset h1 { font-size: 24px !important; }\n}\n";

export function GeistDocsStyles() {
  const [content] = useShadowRootElements(['.md-content']);
  useEffect(() => {
    if (!content) return undefined;
    const style = document.createElement('style');
    style.textContent = styles;
    content.appendChild(style);
    // TechDocs replaces its rendered DOM on navigation. Removal is safe even
    // if the old page has already been detached by the reader.
    return () => style.remove();
  }, [content]);
  return null;
}
