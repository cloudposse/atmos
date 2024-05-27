import React from 'react';


// Import the original mapper
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'; // Import the FontAwesomeIcon component.
import { library } from '@fortawesome/fontawesome-svg-core'; // Import the library component.
import { fab } from '@fortawesome/free-brands-svg-icons'; // Import all brands icons.
import { fas } from '@fortawesome/free-solid-svg-icons'; // Import all solid icons.
import { far } from '@fortawesome/free-regular-svg-icons'; // Import all regular icons.

import { faFile, faFolder, faImage, faLayerGroup, faCube, faGear, faCode, faBarsStaggered } from '@fortawesome/free-solid-svg-icons';


library.add(fab, fas, far); // Add all icons to the library so you can use them without importing them individually.

// Define the mapping of types to icons
const iconMap = {
    file: faFile,
    folder: faFolder,
    image: faImage,
    stack: faLayerGroup,
    component: faCube,
    config: faGear,
    code: faBarsStaggered,
    hcl: faBarsStaggered,
    yaml: faBarsStaggered,
    json: faBarsStaggered,
    // Add more mappings as needed
  };

  // Function to guess the type based on the title
const guessTypeFromTitle = (title) => {
    if (/\.tf\.json$/i.test(title)) {
      return 'code';
    }
    if (/.*stack.*\.ya?ml$/i.test(title)) {
        return 'stack';
    }
    if (/atmos\.ya?ml$/i.test(title)) {
        return 'config';
    }
    if (/\.ya?ml$/i.test(title)) {
        return 'yaml';
    }   
    if (/\.json$/i.test(title)) {
        return 'json';
    }
    if (/\.tf$/i.test(title)) {
        return 'hcl';
    }
    if (/\/$/i.test(title)) {
        return 'folder';
    }
    // Add more patterns as needed
    return 'file'; // Default to 'file'
  };

  
export default function File({ title, className, type, icon, size = '1x', children }) {
    // Determine the icon to use
    const guessedType = type || guessTypeFromTitle(title);
    const selectedIcon = icon || iconMap[guessedType] || faFile;

    return (
        <div className={className}>
            <div className="file">
                <div class="tab">
                    <h1><FontAwesomeIcon icon={selectedIcon} size={size} /><span>{title}</span></h1>
                </div>
                <div className="viewport">{children}</div>
            </div>
        </div>
    );
};

