/**
 * Remark plugin that renders GitHub-style alerts the way GitHub renders them.
 *
 * Transforms:
 *   > [!WARNING]
 *   > Something to watch out for.
 * Into:
 *   <div class="github-alert github-alert-warning">
 *     <p class="github-alert-title">Warning</p>
 *     <p>Something to watch out for.</p>
 *   </div>
 *
 * The content keeps rendering as a regular blockquote on GitHub. On the site it gets
 * GitHub's alert look (colored left bar, icon and title, no tinted box) from the
 * `.github-alert` rules in `src/css/custom.css`, not the Docusaurus admonition theme.
 */

// The marker must end its line (or the paragraph), as on GitHub: `> [!NOTE] text` stays a
// plain blockquote. A paragraph that ends right after the marker, or has a hard break after
// it, leaves the marker alone in its text node, which the `$` alternative covers.
const ALERT_MARKER = /^\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\][ \t]*(?:\r?\n|$)/i;

// GitHub's alert types and the title each one shows.
const ALERT_TITLES = {
  note: 'Note',
  tip: 'Tip',
  important: 'Important',
  warning: 'Warning',
  caution: 'Caution',
};

// Returns the alert type for a blockquote that starts with an alert marker.
// When it does, the marker is stripped from the blockquote's first paragraph.
function extractAlertType(blockquote) {
  const paragraph = blockquote.children && blockquote.children[0];
  if (!paragraph || paragraph.type !== 'paragraph') return undefined;

  const text = paragraph.children && paragraph.children[0];
  if (!text || text.type !== 'text') return undefined;

  const match = ALERT_MARKER.exec(text.value);
  if (!match) return undefined;

  text.value = text.value.slice(match[0].length);
  if (text.value === '') {
    paragraph.children.shift();
  }
  // A marker line ending in two spaces or a backslash parses as a hard break after the marker;
  // drop it so the body does not start with a blank line.
  while (paragraph.children[0] && paragraph.children[0].type === 'break') {
    paragraph.children.shift();
  }
  if (paragraph.children.length === 0) {
    blockquote.children.shift();
  }
  return match[1].toLowerCase();
}

// Builds the title paragraph shown above the alert body.
function alertTitle(type) {
  return {
    type: 'paragraph',
    data: { hName: 'p', hProperties: { className: ['github-alert-title'] } },
    children: [{ type: 'text', value: ALERT_TITLES[type] }],
  };
}

// Walks the tree and turns every alert blockquote into a titled `github-alert` block.
function transform(node) {
  if (!node || !Array.isArray(node.children)) return;

  if (node.type === 'blockquote') {
    const type = extractAlertType(node);
    if (type) {
      node.data = { hName: 'div', hProperties: { className: ['github-alert', `github-alert-${type}`] } };
      node.children.unshift(alertTitle(type));
    }
  }

  node.children.forEach(transform);
}

// Remark plugin entry point: returns the tree transformer that converts GitHub alerts.
function remarkGithubAdmonitions() {
  return (tree) => {
    transform(tree);
  };
}

module.exports = remarkGithubAdmonitions;
