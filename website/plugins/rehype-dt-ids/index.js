/**
 * Rehype plugin to add anchor IDs to <dt> elements for deep-linking.
 *
 * Transforms:
 *   <dt>`vars` (optional)</dt>
 * Into:
 *   <dt id="vars-optional" class="def-term">`vars` (optional)</dt>
 */

const { visit } = require('unist-util-visit');

function slugify(text) {
  return text
    .toLowerCase()
    .replace(/[`'"()]/g, '')
    .replace(/\s+/g, '-')
    .replace(/[^a-z0-9-_.]/g, '')
    .replace(/-+/g, '-')
    .replace(/^-|-$/g, '');
}

// Extract text content from a hast node recursively
function extractText(node) {
  if (!node) return '';
  if (node.type === 'text') return node.value || '';
  if (node.children && Array.isArray(node.children)) {
    return node.children.map(extractText).join('');
  }
  return '';
}

function rehypeDtIds() {
  return (tree) => {
    visit(tree, 'element', (node) => {
      if (node.tagName === 'dt') {
        const text = extractText(node);
        const id = slugify(text);

        if (id) {
          node.properties = node.properties || {};
          node.properties.id = node.properties.id || id;
          node.properties.className = node.properties.className || [];
          if (!node.properties.className.includes('def-term')) {
            node.properties.className.push('def-term');
          }
        }
      }
    });
  };
}

module.exports = rehypeDtIds;
