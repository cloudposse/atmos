import { coreExtensionData } from '@backstage/frontend-plugin-api';
import { createApp } from '@backstage/frontend-defaults';
import catalog from '@backstage/plugin-catalog/alpha';
import catalogGraph from '@backstage/plugin-catalog-graph/alpha';
import catalogImport from '@backstage/plugin-catalog-import/alpha';
import scaffolder from '@backstage/plugin-scaffolder/alpha';
import techdocs from '@backstage/plugin-techdocs/alpha';
import search from '@backstage/plugin-search/alpha';
import apiDocs from '@backstage/plugin-api-docs/alpha';
import org from '@backstage/plugin-org/alpha';
import userSettings from '@backstage/plugin-user-settings/alpha';
import { portalAppModule, portalCatalogModule, portalDocsModule } from './portalModules';

const portalCatalog = catalog.withOverrides({
  extensions: [
    catalog.getExtension('page:catalog').override({
      factory(originalFactory, { inputs, config }) {
        return originalFactory({
          params: {
            loader: async () => {
              const { PortalCatalog } = await import('./PortalCatalog');
              return <PortalCatalog
                filters={<>{inputs.filters.map(filter => filter.get(coreExtensionData.reactElement))}</>}
                pagination={config.pagination}
              />;
            },
          },
        });
      },
    }),
  ],
});

const portalApiDocs = apiDocs.withOverrides({
  extensions: [
    apiDocs.getExtension('page:api-docs').override({
      factory(originalFactory, { config }) {
        return originalFactory({
          params: {
            loader: async () => {
              const { PortalApiExplorer } = await import('./PortalApiExplorer');
              return <PortalApiExplorer initiallySelectedFilter={config.initiallySelectedFilter} />;
            },
          },
        });
      },
    }),
  ],
});

// Explicit features keep this example small and make the installed integrations
// reviewable. Routes and plugin APIs are wired by the frontend extension system.
export default createApp({
  features: [
    portalCatalog, catalogGraph, catalogImport, scaffolder, techdocs, search,
    portalApiDocs, org, userSettings,
    portalAppModule, portalCatalogModule, portalDocsModule,
  ],
});
