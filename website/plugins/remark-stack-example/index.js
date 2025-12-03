/**
 * Remark Stack Example Plugin for Docusaurus.
 * Transforms ```yaml stack-example code blocks into multi-format tabbed views.
 *
 * Usage in MDX:
 * ```yaml stack-example
 * settings:
 *   region: !env AWS_REGION
 *   timeout: 30
 * ```
 *
 * This will generate equivalent YAML, JSON, and HCL versions with proper
 * function syntax translation.
 */
const { convertYamlToFormats } = require('./converter');

/**
 * Walk the AST tree and find nodes of a specific type.
 * This is a simple synchronous implementation to avoid ESM import issues.
 */
function walkTree(node, type, callback, parent = null, index = 0) {
  if (node.type === type) {
    callback(node, index, parent);
  }
  if (node.children) {
    node.children.forEach((child, i) => {
      walkTree(child, type, callback, node, i);
    });
  }
}

module.exports = function remarkStackExample(options = {}) {
  return (tree) => {
    const nodesToReplace = [];

    walkTree(tree, 'code', (node, index, parent) => {
      // Check if this is a stack-example code block.
      const meta = node.meta || '';
      if (!meta.includes('stack-example')) {
        return;
      }

      // Only process YAML code blocks.
      if (node.lang !== 'yaml' && node.lang !== 'yml') {
        return;
      }

      try {
        // Convert YAML to all formats.
        const formats = convertYamlToFormats(node.value);

        // Create JSX element for StackExample component.
        const jsxNode = {
          type: 'mdxJsxFlowElement',
          name: 'StackExample',
          attributes: [
            {
              type: 'mdxJsxAttribute',
              name: 'yaml',
              value: formats.yaml,
            },
            {
              type: 'mdxJsxAttribute',
              name: 'json',
              value: formats.json,
            },
            {
              type: 'mdxJsxAttribute',
              name: 'hcl',
              value: formats.hcl,
            },
          ],
          children: [],
        };

        // Mark for replacement.
        nodesToReplace.push({ parent, index, jsxNode });
      } catch (err) {
        console.warn(`[remark-stack-example] Failed to process code block: ${err.message}`);
        // Leave original block unchanged on error.
      }
    });

    // Replace nodes in reverse order to maintain correct indices.
    for (let i = nodesToReplace.length - 1; i >= 0; i--) {
      const { parent, index, jsxNode } = nodesToReplace[i];
      parent.children.splice(index, 1, jsxNode);
    }
  };
};
