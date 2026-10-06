const fs = require('node:fs');
const path = require('node:path');
const matter = require('gray-matter');

/** Discover explicitly marked component overviews and guides, regardless of folder. */
function componentLibraryItems(docsDir = path.join(__dirname, '../docs')) {
  const overviews = new Map();
  const guides = [];
  function readDirectory(directory) {
    for (const entry of fs.readdirSync(directory, {withFileTypes: true})) {
      const filename = path.join(directory, entry.name);
      if (entry.isDirectory()) {
        readDirectory(filename);
        continue;
      }
      if (!/\.mdx?$/.test(entry.name)) continue;
      const {data} = matter(fs.readFileSync(filename, 'utf8'));
      const metadata = data.component_library;
      const parent = data.component_library_parent;
      if (metadata === undefined && parent === undefined) continue;
      const relative = path.relative(docsDir, filename).split(path.sep).join('/');
      const fail = (message) => { throw new Error(`${relative}: ${message}`); };
      const id = path.posix.join(path.posix.dirname(relative), data.id || entry.name.replace(/\.mdx?$/, ''));
      if (metadata !== undefined) {
        if (!metadata || typeof metadata !== 'object' || Array.isArray(metadata)) fail('component_library must be an object');
        for (const field of ['type', 'label', 'description', 'implementation']) {
          if (typeof metadata[field] !== 'string' || !metadata[field].trim()) fail(`component_library.${field} must be a nonempty string`);
        }
        if (!Number.isFinite(metadata.position)) fail('component_library.position must be a number');
        for (const field of ['reference', 'native']) {
          if (metadata[field] !== undefined && typeof metadata[field] !== 'boolean') fail(`component_library.${field} must be a boolean`);
        }
        if (parent !== undefined) fail('a component overview cannot also be a child guide');
        if (overviews.has(metadata.type)) fail(`duplicate component type ${metadata.type}`);
        if (metadata.reference && (typeof data.slug !== 'string' || !data.slug.startsWith('/'))) fail('shared component overviews require an absolute slug');
        overviews.set(metadata.type, {id, metadata, slug: data.slug});
      } else {
        if (typeof parent !== 'string' || !parent.trim()) fail('component_library_parent must be a nonempty string');
        guides.push({parent, filename: relative, position: data.sidebar_position ?? Infinity,
          item: {type: 'doc', id, label: data.sidebar_label || data.title || entry.name.replace(/\.mdx?$/, '')}});
      }
    }
  }
  readDirectory(docsDir);
  for (const guide of guides) {
    if (!overviews.has(guide.parent)) throw new Error(`${guide.filename}: unknown component_library_parent ${guide.parent}`);
  }
  return [...overviews.values()]
    .sort((a, b) => a.metadata.position - b.metadata.position || a.metadata.label.localeCompare(b.metadata.label))
    .map(({id, metadata, slug}) => {
      const children = guides.filter(guide => guide.parent === metadata.type)
        .sort((a, b) => a.position - b.position || a.item.label.localeCompare(b.item.label))
        .map(guide => guide.item);
      const common = {label: metadata.label, customProps: {
        componentType: {id: metadata.type, native: metadata.native !== false, implementation: metadata.implementation, description: metadata.description},
      }};
      if (metadata.reference) {
        if (children.length) throw new Error(`${id}: shared component overviews cannot own child guides`);
        return {...common, type: 'link', href: slug, description: metadata.description, customProps: {...common.customProps, navigationReference: true}};
      }
      return children.length
        ? {...common, type: 'category', description: metadata.description, collapsed: true, link: {type: 'doc', id}, items: children}
        : {...common, type: 'doc', id};
    });
}

module.exports = {componentLibraryItems};
