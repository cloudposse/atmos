// MDX → plain-Markdown normalizer for atmos.tools docs.
//
// Walks the MDAST produced by remark-parse + remark-mdx and rewrites custom
// JSX components (Intro, Terminal, Tabs/TabItem, Note, File, dl/dt/dd, …) into
// portable Markdown so the output is useful to humans and LLMs without the
// React runtime. Unknown JSX falls through to "unwrap and keep children".

import { unified } from 'unified';
import remarkParse from 'remark-parse';
import remarkMdx from 'remark-mdx';
import remarkStringify from 'remark-stringify';

// Handlers that must run BEFORE recursing into children.
// Most components are post-order (we transform children first, then act on the
// result). A few — like `<dl>` — need to inspect their raw `<dt>`/`<dd>`
// children before those get transformed away.
const PREORDER = new Set(['dl', 'EmbedFile', 'EmbedExample', 'RemoteFile']);

// Extract a JSX attribute value as a plain string. Returns '' for missing
// attrs and for expression-valued attrs we can't statically evaluate.
function attr(node, name) {
  if (!node.attributes) return '';
  const a = node.attributes.find((x) => x && x.type === 'mdxJsxAttribute' && x.name === name);
  if (!a) return '';
  if (typeof a.value === 'string') return a.value;
  if (a.value && typeof a.value === 'object' && 'value' in a.value) {
    const raw = a.value.value;
    if (typeof raw === 'string') {
      const trimmed = raw.trim();
      if ((trimmed.startsWith('"') && trimmed.endsWith('"')) ||
          (trimmed.startsWith("'") && trimmed.endsWith("'")) ||
          (trimmed.startsWith('`') && trimmed.endsWith('`'))) {
        return trimmed.slice(1, -1);
      }
      return raw;
    }
  }
  return '';
}

// Concatenate all plain text under a node (text, inlineCode, code).
function childrenText(node) {
  let out = '';
  function walk(n) {
    if (!n) return;
    if (n.type === 'text') out += n.value;
    else if (n.type === 'inlineCode') out += n.value;
    else if (n.type === 'code') out += n.value;
    if (n.children) n.children.forEach(walk);
  }
  if (node.children) node.children.forEach(walk);
  return out;
}

// Infer fenced-code language from a file name.
function langFromName(name) {
  if (!name) return '';
  const ext = name.split('.').pop().toLowerCase();
  return ({
    yaml: 'yaml', yml: 'yaml',
    hcl: 'hcl', tf: 'hcl', tfvars: 'hcl',
    go: 'go', js: 'js', mjs: 'js', cjs: 'js',
    ts: 'ts', tsx: 'tsx', jsx: 'jsx',
    sh: 'shell', bash: 'shell', zsh: 'shell',
    md: 'markdown', mdx: 'markdown',
    json: 'json', toml: 'toml', xml: 'xml',
    py: 'python', rb: 'ruby', rs: 'rust',
    dockerfile: 'dockerfile',
  })[ext] || '';
}

// MDAST node constructors.
const text = (v) => ({ type: 'text', value: v });
const paragraph = (children) => ({ type: 'paragraph', children });
const heading = (depth, children) => ({ type: 'heading', depth, children });
const blockquote = (children) => ({ type: 'blockquote', children });
const code = (lang, value) => ({ type: 'code', lang: lang || null, value: value || '' });
const list = (ordered, children) => ({ type: 'list', ordered, spread: false, children });
const listItem = (children) => ({ type: 'listItem', spread: false, children });
const link = (url, children) => ({ type: 'link', url, title: null, children });
const strong = (children) => ({ type: 'strong', children });
const emphasis = (children) => ({ type: 'emphasis', children });
const inlineCode = (value) => ({ type: 'inlineCode', value });

// Marketing/card-style components — emit title + body + CTA link, drop chrome.
function cardHandler(node) {
  const title = attr(node, 'title') || attr(node, 'label') || attr(node, 'name') || '';
  const href = attr(node, 'href') || attr(node, 'to') || attr(node, 'link') || attr(node, 'url') || '';
  const ctaLabel = attr(node, 'ctaLabel') || attr(node, 'cta') || (href ? 'Read more' : '');
  const out = [];
  if (title) out.push(paragraph([strong([text(title)])]));
  if (node.children && node.children.length) out.push(...node.children);
  if (href) out.push(paragraph([link(href, [text(ctaLabel || href)])]));
  return out.length ? out : null;
}

function embedHandler(node) {
  const src = attr(node, 'from') || attr(node, 'src') || attr(node, 'url') || attr(node, 'file') || '';
  if (!src) return null;
  const lang = langFromName(src);
  return paragraph([
    strong([text('Embed: ')]),
    inlineCode(src),
    text(lang ? ` (${lang})` : ''),
  ]);
}

// Recursively collect every JSX child with the given name, regardless of how
// deeply remark-parse wrapped them in paragraph/text nodes. Order-preserving.
function collectJsxChildren(node, names) {
  const out = [];
  function walk(n) {
    if (!n) return;
    if ((n.type === 'mdxJsxFlowElement' || n.type === 'mdxJsxTextElement') && names.has(n.name)) {
      out.push(n);
      return; // don't descend into matched nodes
    }
    if (Array.isArray(n.children)) n.children.forEach(walk);
  }
  if (Array.isArray(node.children)) node.children.forEach(walk);
  return out;
}

// Pre-order handler: pair dt+dd into bulleted definition list items.
// remark-parse may wrap them inside a paragraph, so we collect by name
// rather than iterating direct children.
function dlHandler(node, transformChildList) {
  const dtDdNodes = collectJsxChildren(node, new Set(['dt', 'dd']));
  const items = [];
  let currentDt = null;
  for (const child of dtDdNodes) {
    if (child.name === 'dt') {
      currentDt = transformChildList(child.children || []);
    } else if (child.name === 'dd') {
      const ddChildren = transformChildList(child.children || []);
      const itemChildren = [
        paragraph(currentDt && currentDt.length ? [strong(currentDt)] : [text('—')]),
      ];
      if (ddChildren.length) itemChildren.push(...ddChildren);
      items.push(listItem(itemChildren));
      currentDt = null;
    }
  }
  return items.length ? list(false, items) : null;
}

// Post-order JSX handlers. Each receives the (already-transformed-children)
// node and returns: an MDAST node, an array of MDAST nodes, or null (drop).
const POST_HANDLERS = {
  Intro: (n) => n.children,
  Screengrab: () => null,
  DocCardList: () => null,
  Tabs: (n) => n.children,
  TabItem: (n) => {
    const label = attr(n, 'label') || attr(n, 'value') || '';
    const out = [];
    if (label) out.push(heading(3, [text(label)]));
    if (n.children && n.children.length) out.push(...n.children);
    return out;
  },
  Terminal: (n) => {
    const body = childrenText(n).trim();
    if (!body) return null;
    return code(attr(n, 'language') || 'shell', body);
  },
  File: (n) => {
    const name = attr(n, 'name') || attr(n, 'title') || '';
    const lang = attr(n, 'language') || langFromName(name);
    const body = childrenText(n).trim();
    const out = [];
    if (name) {
      out.push(paragraph([strong([text('File:')]), text(' '), inlineCode(name)]));
    }
    if (body) {
      out.push(code(lang, body));
    } else if (n.children && n.children.length) {
      // No raw text — keep transformed children (might be MDX-derived content).
      out.push(...n.children);
    }
    return out.length ? out : null;
  },
  Note: (n) => blockquote([
    paragraph([strong([text('Note')])]),
    ...((n.children) || []),
  ]),
  Experimental: (n) => blockquote([
    paragraph([text('⚠️ Experimental')]),
    ...((n.children) || []),
  ]),
  KeyPoints: (n) => blockquote([
    paragraph([strong([text('Key points')])]),
    ...((n.children) || []),
  ]),
  Step: (n) => listItem(n.children && n.children.length ? n.children : [paragraph([text('')])]),
  StepNumber: (n) => listItem(n.children && n.children.length ? n.children : [paragraph([text('')])]),
  FAQItem: (n) => {
    const q = attr(n, 'question') || attr(n, 'title') || '';
    const out = [];
    if (q) out.push(heading(3, [text(q)]));
    if (n.children && n.children.length) out.push(...n.children);
    return out;
  },
  PillBox: (n) => {
    if (!n.children || !n.children.length) return null;
    // Inline-flatten: pull text/inline children out of any wrapping paragraph.
    const inlines = [];
    for (const c of n.children) {
      if (c.type === 'paragraph' && Array.isArray(c.children)) inlines.push(...c.children);
      else inlines.push(c);
    }
    return paragraph([emphasis(inlines)]);
  },
  DemoVideo: (n) => {
    const title = attr(n, 'title') || 'Video';
    const url = attr(n, 'url') || attr(n, 'src') || '';
    if (!url) return paragraph([emphasis([text(`[Video: ${title}]`)])]);
    return paragraph([link(url, [text(`[Video] ${title}`)])]);
  },
  LatestRelease: () => paragraph([emphasis([text('(see latest release)')])]),
  // Marketing / card chrome.
  ActionCard: cardHandler,
  Card: cardHandler,
  PrimaryCTA: cardHandler,
  FeatureCard: cardHandler,
  JourneyCard: cardHandler,
  UseCaseCard: cardHandler,
  FeatureGrid: (n) => n.children,
  CardGroup: (n) => n.children,
  // Slide deck components: keep <SlideNotes> body, drop the visual rest.
  Slide: (n) => n.children,
  SlideContent: () => null,
  SlideTitle: () => null,
  SlideList: () => null,
  SlideNotes: (n) => n.children,
  // dt/dd are processed inside dl's pre-order handler — drop any stragglers.
  dt: () => null,
  dd: () => null,
};

// Pre-order JSX handlers. These get the raw, un-transformed node and a
// `transformChildList` helper for recursing into selected children.
const PRE_HANDLERS = {
  dl: dlHandler,
  EmbedFile: embedHandler,
  EmbedExample: embedHandler,
  RemoteFile: embedHandler,
};

function isJsx(node) {
  return node && (node.type === 'mdxJsxFlowElement' || node.type === 'mdxJsxTextElement');
}

// Drop ESM imports and bare {expression} nodes.
function isStripped(node) {
  return node && (
    node.type === 'mdxjsEsm' ||
    node.type === 'mdxFlowExpression' ||
    node.type === 'mdxTextExpression'
  );
}

function transformChildList(children) {
  if (!Array.isArray(children)) return [];
  const out = [];
  for (const child of children) {
    if (isStripped(child)) continue;
    const r = processNode(child);
    if (r === null || r === undefined) continue;
    if (Array.isArray(r)) out.push(...r);
    else out.push(r);
  }
  return out;
}

function processNode(node) {
  if (!node) return null;
  if (isStripped(node)) return null;

  // Pre-order JSX: handler inspects raw children.
  if (isJsx(node) && PREORDER.has(node.name)) {
    const handler = PRE_HANDLERS[node.name];
    if (handler) return handler(node, transformChildList);
  }

  // Recurse into children for everything else.
  if (Array.isArray(node.children)) {
    node.children = transformChildList(node.children);
  }

  // Post-order JSX: handler sees already-transformed children.
  if (isJsx(node)) {
    const handler = POST_HANDLERS[node.name];
    if (handler) return handler(node);
    // Default: unwrap unknown component, keep its children inline.
    return (node.children && node.children.length) ? node.children : null;
  }

  return node;
}

// remark plugin: rewrite the MDAST in place.
function remarkNormalizeMdx() {
  return (tree) => {
    if (Array.isArray(tree.children)) {
      tree.children = transformChildList(tree.children);
    }
    return tree;
  };
}

/**
 * Normalize an MDX source string to plain Markdown.
 *
 * Strips frontmatter expectations (caller should pass post-matter body),
 * drops imports/expressions, and rewrites known JSX components per the
 * handler tables above. Unknown components are unwrapped.
 *
 * @param {string} source - MDX/Markdown source (without frontmatter).
 * @returns {Promise<string>} - Normalized Markdown.
 */
// MDX 3 forbids HTML comments (uses {/* … */} instead). Strip them so we can
// still normalize docs and blog posts that follow the Docusaurus convention
// of `<!--truncate-->` markers.
function stripHtmlComments(source) {
  return source.replace(/<!--[\s\S]*?-->/g, '');
}

export async function normalizeMdxToMarkdown(source) {
  if (typeof source !== 'string' || !source.length) return '';
  const file = await unified()
    .use(remarkParse)
    .use(remarkMdx)
    .use(remarkNormalizeMdx)
    .use(remarkStringify, {
      bullet: '-',
      fences: true,
      listItemIndent: 'one',
      rule: '-',
      emphasis: '_',
      strong: '*',
    })
    .process(stripHtmlComments(source));
  return String(file);
}
