import { Box, ThemeProvider, Typography, useMediaQuery, useTheme } from '@material-ui/core';
import CodeIcon from '@material-ui/icons/Code';
import { Content, CreateButton, SupportButton } from '@backstage/core-components';
import { useRouteRef } from '@backstage/core-plugin-api';
import { HeaderPage } from '@backstage/ui';
import { apiDocsPlugin } from '@backstage/plugin-api-docs';
import { CatalogTable } from '@backstage/plugin-catalog';
import { catalogEntityCreatePermission } from '@backstage/plugin-catalog-common/alpha';
import { usePermission } from '@backstage/plugin-permission-react';
import {
  CatalogFilterLayout, EntityKindPicker, EntityLifecyclePicker, EntityListProvider,
  EntityOwnerPicker, EntityTagPicker, EntityTypePicker, UserListPicker,
} from '@backstage/plugin-catalog-react';
import type { UserListFilterKind } from '@backstage/plugin-catalog-react';

// Keep the native API page extension and catalog APIs; customize its presentation.
export function PortalApiExplorer({ initiallySelectedFilter = 'all' }: {
  initiallySelectedFilter?: UserListFilterKind;
}) {
  const theme = useTheme();
  const mobile = useMediaQuery(theme.breakpoints.down('xs'));
  const wide = useMediaQuery(theme.breakpoints.up('lg'));
  const registerApi = useRouteRef(apiDocsPlugin.externalRoutes.registerApi);
  const { allowed } = usePermission({ permission: catalogEntityCreatePermission });
  const columns = [
    CatalogTable.columns.createNameColumn({ defaultKind: 'API' }),
    CatalogTable.columns.createSpecTypeColumn(),
    ...(!mobile ? [
      CatalogTable.columns.createOwnerColumn(),
      CatalogTable.columns.createSpecLifecycleColumn(),
    ] : []),
    ...(wide ? [{
      ...CatalogTable.columns.createMetadataDescriptionColumn(),
      render: ({ entity }: { entity: { metadata: { description?: string } } }) => (
        <Typography variant="body2">{entity.metadata.description || '—'}</Typography>
      ),
    }] : []),
  ];

  return (
    <main aria-label="API explorer">
      <HeaderPage title="APIs" customActions={<>
        {allowed && registerApi && <CreateButton title="Register Existing API" to={registerApi()} />}
        <SupportButton>Discover API definitions and their owners in the catalog.</SupportButton>
      </>} />
      <Content>
        <EntityListProvider>
          <CatalogFilterLayout>
            <CatalogFilterLayout.Filters>
              <EntityKindPicker initialFilter="api" hidden />
              <EntityTypePicker />
              <UserListPicker initialFilter={initiallySelectedFilter} />
              <EntityOwnerPicker />
              <EntityLifecyclePicker />
              <EntityTagPicker />
            </CatalogFilterLayout.Filters>
            <CatalogFilterLayout.Content>
              {/* Match the catalog's correction for the upstream nested heading. */}
              <ThemeProvider theme={{ ...theme, props: { ...theme.props, MuiTypography: {
                ...theme.props?.MuiTypography, variantMapping: { h5: 'div' },
              } } }}>
                <CatalogTable
                  columns={columns}
                  tableOptions={{ sorting: false, padding: 'dense' }}
                  emptyContent={
                    <Box py={6} px={3} textAlign="center" color="text.secondary">
                      <CodeIcon aria-hidden="true" />
                      <Typography variant="subtitle1" color="textPrimary">No APIs to display</Typography>
                      <Typography variant="body2">
                        {allowed && registerApi
                          ? 'Try different filters or register an API to get started.'
                          : 'Try different filters to find an API.'}
                      </Typography>
                    </Box>
                  }
                />
              </ThemeProvider>
            </CatalogFilterLayout.Content>
          </CatalogFilterLayout>
        </EntityListProvider>
      </Content>
    </main>
  );
}
