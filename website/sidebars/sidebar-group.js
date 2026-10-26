const fs = require('node:fs');
const path = require('node:path');
const matter = require('gray-matter');

/** Build a navigation group from document front matter, independent of folders. */
function sidebarGroupItems(group, {
  docsDir = path.join(__dirname, '../docs'),
  customPropsForDoc = () => ({}),
} = {}) {
  const documents = new Map();
  function readDirectory(directory) {
    for (const entry of fs.readdirSync(directory, {withFileTypes: true}).sort((a, b) => a.name.localeCompare(b.name))) {
      const filename = path.join(directory, entry.name);
      if (entry.isDirectory()) {
        readDirectory(filename);
        continue;
      }
      if (!/\.mdx?$/.test(entry.name)) continue;
      const {data} = matter(fs.readFileSync(filename, 'utf8'));
      if (data.sidebar_group !== group) continue;
      const relative = path.relative(docsDir, filename).split(path.sep).join('/');
      const fail = message => { throw new Error(`${relative}: ${message}`); };
      const id = path.posix.join(path.posix.dirname(relative), data.id || entry.name.replace(/\.mdx?$/, ''));
      const label = data.sidebar_label || data.title;
      if (typeof label !== 'string' || !label.trim()) fail('sidebar_label or title is required');
      if (data.sidebar_position !== undefined && !Number.isFinite(data.sidebar_position)) fail('sidebar_position must be a number');
      if (data.sidebar_parent !== undefined && (typeof data.sidebar_parent !== 'string' || !data.sidebar_parent.trim())) fail('sidebar_parent must be a document ID');
      if (data.sidebar_reference !== undefined && typeof data.sidebar_reference !== 'boolean') fail('sidebar_reference must be a boolean');
      if (data.sidebar_reference && (typeof data.slug !== 'string' || !/^\/(?!\/)/.test(data.slug))) fail('shared documents require an absolute slug');
      if (documents.has(id)) fail(`duplicate document ID ${id}`);
      documents.set(id, {id, label, data, fail, props: customPropsForDoc(data, relative)});
    }
  }
  readDirectory(docsDir);
  // Check every ancestry chain, including cycles disconnected from the root.
  for (const doc of documents.values()) {
    const seen = new Set([doc.id]);
    let parent = doc.data.sidebar_parent;
    while (parent) {
      if (!documents.has(parent)) doc.fail(`unknown sidebar_parent ${parent} in ${group}`);
      if (seen.has(parent)) doc.fail(`cyclic sidebar_parent ${parent}`);
      seen.add(parent);
      parent = documents.get(parent).data.sidebar_parent;
    }
  }
  function childrenOf(parent) {
    return [...documents.values()].filter(doc => doc.data.sidebar_parent === parent)
      .sort((a, b) => (a.data.sidebar_position ?? Infinity) - (b.data.sidebar_position ?? Infinity) || a.label.localeCompare(b.label))
      .map(({id, label, data, fail, props}) => {
        const children = childrenOf(id);
        const common = {label, customProps: props};
        if (data.sidebar_class_name) common.className = data.sidebar_class_name;
        if (data.sidebar_reference) {
          if (children.length) fail('shared documents cannot own child guides');
          return {...common, type: 'link', href: data.slug, description: data.description,
            customProps: {...props, navigationReference: true}};
        }
        return children.length
          ? {...common, type: 'category', description: data.description, collapsed: true, link: {type: 'doc', id}, items: children}
          : {...common, type: 'doc', id};
      });
  }
  return childrenOf(undefined);
}

module.exports = {sidebarGroupItems};
