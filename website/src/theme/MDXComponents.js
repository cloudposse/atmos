import React from 'react';
// Import the original mapper
import MDXComponents from '@theme-original/MDXComponents';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'; // Import the FontAwesomeIcon component.
import { library } from '@fortawesome/fontawesome-svg-core'; // Import the library component.
import { fab } from '@fortawesome/free-brands-svg-icons'; // Import all brands icons.
import { fas } from '@fortawesome/free-solid-svg-icons'; // Import all solid icons.
import { far } from '@fortawesome/free-regular-svg-icons'; // Import all regular icons.
import StackExample from '@site/src/components/StackExample'; // Multi-format code example component.

library.add(fab, fas, far); // Add all icons to the library so you can use them without importing them individually.

export default {
  // Re-use the default mapping
  ...MDXComponents,
  FAIcon: FontAwesomeIcon, // Make the FontAwesomeIcon component available in MDX as <icon />.
  StackExample, // Multi-format stack configuration examples (YAML/JSON/HCL).
};
