import { createFrontendModule } from '@backstage/frontend-plugin-api';
import { NavContentBlueprint, SignInPageBlueprint, ThemeBlueprint } from '@backstage/plugin-app-react';
import { EntityContentBlueprint } from '@backstage/plugin-catalog-react/alpha';
import { AddonBlueprint } from '@backstage/plugin-techdocs-react/alpha';
import { TechDocsAddonLocations } from '@backstage/plugin-techdocs-react';
import { geistThemes } from './themes';
import { PortalNav } from './PortalNav';
import { GeistDocsStyles } from './GeistDocs';

export const portalAppModule = createFrontendModule({
  pluginId: 'app',
  extensions: [
    ...geistThemes.map(theme => ThemeBlueprint.make({ name: `geist-${theme.id}`, params: { theme } })),
    NavContentBlueprint.make({ params: { component: PortalNav } }),
    SignInPageBlueprint.make({
      params: {
        loader: async () => {
          const { SignInPage } = await import('@backstage/core-components');
          return props => <SignInPage {...props} auto providers={['guest']} />;
        },
      },
    }),
  ],
});

export const portalCatalogModule = createFrontendModule({
  pluginId: 'catalog',
  extensions: [
    EntityContentBlueprint.make({
      name: 'gitea-actions',
      params: {
        path: '/ci-cd',
        title: 'CI/CD',
        filter: 'kind:component',
        loader: async () => {
          const { GiteaActions } = await import('./GiteaActions');
          return <GiteaActions />;
        },
      },
    }),
  ],
});

export const portalDocsModule = createFrontendModule({
  pluginId: 'techdocs',
  extensions: [
    AddonBlueprint.make({
      name: 'geist',
      params: {
        name: 'Geist typography',
        location: TechDocsAddonLocations.Content,
        component: GeistDocsStyles,
      },
    }),
  ],
});
