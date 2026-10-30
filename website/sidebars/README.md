# Metadata-driven sidebar groups

`sidebar-group.js` discovers documents by front matter, independent of their
folders. `sidebar_group`, `sidebar_parent`, and `sidebar_reference` are local
conventions implemented by this helper. Labels, ordering, descriptions, and
`sidebar_class_name` reuse existing Docusaurus front matter:

```yaml
sidebar_group: component-library
sidebar_label: Example
sidebar_position: 10
description: What this component helps users do
```

Call `sidebarGroupItems('group-name')` to generate another section. Documents
without that group are excluded. Ties in position are sorted by label. A group
selects the documents; the sidebar definition still controls its section heading.
Parents with children become collapsible categories linked to their overview.

## Nested guides and shared pages

Add `sidebar_parent` with the parent document's existing Docusaurus ID to nest a
guide. Both documents belong to the same group:

```yaml
sidebar_group: component-library
sidebar_parent: components/terraform/terraform
sidebar_label: Stack Configuration
sidebar_position: 1
```

For a page owned by another section, use `sidebar_reference: true` and its
existing absolute `slug`. The generator creates a cross-link, preserving the
canonical navigation and breadcrumbs. Shared pages cannot own child guides.
Invalid parents, cycles, and duplicate document IDs fail the build.

## Component Library

`component-library.js` uses the shared generator and adds these facts from native
component overview pages:

```yaml
component_type: example
component_implementation: Example source files
```

The Component Library sidebar, component table, native-type count, and Next Steps
cards all use this generated list. Custom Components belongs to the sidebar group
without declaring a native `component_type`. Child guides and configuration
fields do not enter the native-type table.

Native component overviews belong in `docs/components/` so their sidebar links
stay in the Component Library. Link from each overview to its detailed stack
configuration page in `docs/stacks/components/`.

Adding an overview or child guide requires no edit to `sidebars.js`. Run
`pnpm --dir website test:navigation` to validate the generator and navigation.
