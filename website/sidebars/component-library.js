const {sidebarGroupItems} = require('./sidebar-group');

/** Add component-specific overview facts to the shared navigation generator. */
function componentLibraryItems(docsDir) {
  const types = new Set();
  return sidebarGroupItems('component-library', {
    docsDir,
    customPropsForDoc(data, filename) {
      if (data.component_type === undefined) return {};
      const fail = message => { throw new Error(`${filename}: ${message}`); };
      for (const field of ['component_type', 'component_implementation', 'description']) {
        if (typeof data[field] !== 'string' || !data[field].trim()) fail(`${field} must be a nonempty string`);
      }
      if (data.sidebar_parent) fail('component overviews cannot also be child guides');
      if (types.has(data.component_type)) fail(`duplicate component type ${data.component_type}`);
      types.add(data.component_type);
      return {componentType: {
        id: data.component_type,
        native: true,
        implementation: data.component_implementation,
        description: data.description,
      }};
    },
  });
}

module.exports = {componentLibraryItems};
