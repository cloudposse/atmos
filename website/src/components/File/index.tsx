import React from 'react';
import './index.css';
import Link from '@docusaurus/Link';
import { useLocation } from '@docusaurus/router';
import useGlobalData from '@docusaurus/useGlobalData';
import { SiYaml } from 'react-icons/si';
import { RiExternalLinkLine, RiSideBarLine } from 'react-icons/ri';

// Import the original mapper
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'; // Import the FontAwesomeIcon component.
import { library } from '@fortawesome/fontawesome-svg-core'; // Import the library component.
import { fab } from '@fortawesome/free-brands-svg-icons'; // Import all brands icons.
import { fas } from '@fortawesome/free-solid-svg-icons'; // Import all solid icons.
import { far } from '@fortawesome/free-regular-svg-icons'; // Import all regular icons.

import {
    faFile,
    faFolder,
    faImage,
    faLayerGroup,
    faCube,
    faGear,
    faCode,
    faBarsStaggered,
} from '@fortawesome/free-solid-svg-icons';
import { findExampleByName, findNodeByPath } from '@site/src/components/FileBrowser/utils';
import { getQuickStartExampleConfig } from '@site/src/components/QuickStartExampleDrawer/routeMap';


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

const reactIconMap = {
    yaml: SiYaml,
};

// Function to guess the type based on the title
const guessTypeFromTitle = (title) => {
    if (/\.tf\.json$/i.test(title)) {
      return 'code';
    }
    if (/atmos\.ya?ml$/i.test(title)) {
        return 'yaml';
    }
    if (/.*stack.*\.ya?ml$/i.test(title)) {
        return 'stack';
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

const getExampleRelativePath = (title, exampleName) => {
    if (!title || /\/$/i.test(title)) {
        return null;
    }

    if (title.startsWith('examples/')) {
        return title.slice('examples/'.length);
    }

    if (title.startsWith(`${exampleName}/`)) {
        return title;
    }

    return `${exampleName}/${title}`;
};

export default function File({ title, className, type, icon, size = '1x', children }) {
    // Determine the icon to use
    const guessedType = type || guessTypeFromTitle(title);
    const selectedIcon = icon || iconMap[guessedType] || faFile;
    const ReactIcon = !icon ? reactIconMap[guessedType] : null;
    const { pathname } = useLocation();
    const config = getQuickStartExampleConfig(pathname);
    const globalData = useGlobalData();
    const fileBrowserData = globalData['file-browser']?.['examples'];
    const example = config && fileBrowserData?.examples
        ? findExampleByName(fileBrowserData.examples, config.exampleName)
        : undefined;
    const examplePath = config ? getExampleRelativePath(title, config.exampleName) : null;
    const exampleNode = examplePath && example ? findNodeByPath(example.root, examplePath) : null;
    const exampleHref = exampleNode && fileBrowserData?.options?.routeBasePath
        ? `${fileBrowserData.options.routeBasePath}/${examplePath}`
        : null;
    const openExampleDrawer = () => {
        if (!examplePath || typeof window === 'undefined') {
            return;
        }

        window.dispatchEvent(new CustomEvent('quick-start-example-drawer:open', {
            detail: { path: examplePath },
        }));
    };

    return (
        <div className={className}>
            <div className="file">
                <div className="file-header">
                    <div className="file-title">
                        {ReactIcon ? (
                            <ReactIcon
                                className={`file-type-icon file-type-icon--${guessedType}`}
                                aria-hidden="true"
                            />
                        ) : (
                            <FontAwesomeIcon icon={selectedIcon} size={size} />
                        )}
                        {exampleHref ? (
                            <Link to={exampleHref} title={`Open ${title} in the example browser`}>
                                {title}
                            </Link>
                        ) : (
                            <span>{title}</span>
                        )}
                    </div>
                    {exampleNode && exampleHref && (
                        <div className="file-actions">
                            <Link
                                to={exampleHref}
                                className="file-action-button"
                                title={`Open ${title} in the example browser`}
                                aria-label={`Open ${title} in the example browser`}
                            >
                                <RiExternalLinkLine aria-hidden="true" />
                            </Link>
                            <button
                                type="button"
                                className="file-action-button"
                                onClick={openExampleDrawer}
                                title={`Open ${title} in the example drawer`}
                                aria-label={`Open ${title} in the example drawer`}
                            >
                                <RiSideBarLine aria-hidden="true" />
                            </button>
                        </div>
                    )}
                </div>
                <div className="viewport">
                    {children}
                </div>
            </div>
        </div>
    );
};
