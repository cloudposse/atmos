/**
 * Remark plugin that renders GitHub-style alerts as Docusaurus admonitions.
 *
 * Transforms:
 *   > [!WARNING]
 *   > Something to watch out for.
 * Into the same node Docusaurus produces for:
 *   :::warning
 *   Something to watch out for.
 *   :::
 *
 * The content keeps rendering as a regular blockquote on GitHub, and on the site
 * it picks up the theme's admonition styling.
 */

// The marker must end its line (or the paragraph), as on GitHub: `> [!NOTE] text` stays a
// plain blockquote. A paragraph that ends right after the marker, or has a hard break after
// it, leaves the marker alone in its text node, which the `$` alternative covers.
const ALERT_MARKER = /^\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\][ \t]*(?:\r?\n|$)/i;

// Maps a GitHub alert type to a Docusaurus admonition type. GitHub's CAUTION is the
// severe (red) alert, which Docusaurus calls `danger` (its `caution` is a warning alias).
const ADMONITION_TYPES = {
  note: 'note',
  tip: 'tip',
  important: 'important',
  warning: 'warning',
  caution: 'danger',
};

// Returns the admonition type for a blockquote that starts with an alert marker.
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
  return ADMONITION_TYPES[match[1].toLowerCase()];
}

// Walks the tree and tags every alert blockquote so it renders as an admonition.
function transform(node) {
  if (!node || !Array.isArray(node.children)) return;

  if (node.type === 'blockquote') {
    const type = extractAlertType(node);
    if (type) {
      node.data = { hName: 'admonition', hProperties: { type } };
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
