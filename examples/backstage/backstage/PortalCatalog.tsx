import { ComponentProps } from 'react';
import { ThemeProvider, Typography, useMediaQuery, useTheme } from '@material-ui/core';
import StarIcon from '@material-ui/icons/Star';
import StarBorderIcon from '@material-ui/icons/StarBorder';
import { CatalogTable } from '@backstage/plugin-catalog';
import { CatalogIndexPage } from '@backstage/plugin-catalog/alpha';
import { useStarredEntities } from '@backstage/plugin-catalog-react';

// Customize the native frontend page's table, retaining its header, route,
// filters, and pagination. No legacy routable extensions or app conversion.
export function PortalCatalog(props: ComponentProps<typeof CatalogIndexPage>) {
  const theme = useTheme();
  const mobile = useMediaQuery(theme.breakpoints.down('xs'));
  const { isStarredEntity, toggleStarredEntity } = useStarredEntities();
  const columns = [
    CatalogTable.columns.createNameColumn({ defaultKind: 'Component' }),
    ...(!mobile ? [CatalogTable.columns.createOwnerColumn(), CatalogTable.columns.createSpecTypeColumn()] : []),
    CatalogTable.columns.createSpecLifecycleColumn(),
    ...(!mobile ? [{
      ...CatalogTable.columns.createMetadataDescriptionColumn(),
      cellStyle: { minWidth: 220, whiteSpace: 'normal' as const },
      render: ({ entity }: { entity: { metadata: { description?: string } } }) => (
        <Typography variant="body2">{entity.metadata.description || '—'}</Typography>
      ),
    }] : []),
  ];
  return (
    <main aria-label="Service catalog">
      {/* The upstream toolbar nests its h2 title inside an h5 Typography.
          Scope this semantic correction to the catalog; keep its visual style. */}
      <ThemeProvider theme={{ ...theme, props: { ...theme.props, MuiTypography: {
        ...theme.props?.MuiTypography, variantMapping: { h5: 'div' },
      } } }}>
        <CatalogIndexPage
          {...props}
          columns={columns}
          actions={[
            ({ entity }) => ({
              icon: () => isStarredEntity(entity) ? <StarIcon fontSize="small" /> : <StarBorderIcon fontSize="small" />,
              tooltip: isStarredEntity(entity) ? 'Remove from favorites' : 'Add to favorites',
              onClick: () => toggleStarredEntity(entity),
            }),
          ]}
          // Upstream sorting renders nested buttons; filters/search remain available.
          tableOptions={{ sorting: false, padding: 'dense' }}
        />
      </ThemeProvider>
    </main>
  );
}
