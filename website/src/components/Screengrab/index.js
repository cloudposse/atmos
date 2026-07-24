import React from 'react';
import Terminal from '@site/src/components/Terminal';
import './styles.css';

import migrateHelp from '@site/src/components/Screengrabs/atmos-terraform-migrate--help.html';
import migrateApplyHelp from '@site/src/components/Screengrabs/atmos-terraform-migrate-apply--help.html';
import migrateListHelp from '@site/src/components/Screengrabs/atmos-terraform-migrate-list--help.html';
import migratePlanHelp from '@site/src/components/Screengrabs/atmos-terraform-migrate-plan--help.html';

const screengrabs = {
    'atmos-terraform-migrate--help': migrateHelp,
    'atmos-terraform-migrate-apply--help': migrateApplyHelp,
    'atmos-terraform-migrate-list--help': migrateListHelp,
    'atmos-terraform-migrate-plan--help': migratePlanHelp,
};

export default function Screengrab({ title, slug }) {
    const html = screengrabs[slug];

    if (!html) {
        return null;
    }

    return (
        <Terminal title={title} className="screengrab">
            <pre className="screengrab__content" dangerouslySetInnerHTML={{ __html: html }} />
        </Terminal>
    );
}
