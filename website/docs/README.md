# Docs folder

The `/docs` folder houses all markdown files. 
The page structure loosely maps to the routing on the site since paths can be changed in the frontmatter.

## Versioning

This folder can also contain components, assets, and whatever else is meant to be versioned when the docusaurus versioning script is run. For example,
if there is a page component that is only relevant to the `layout` section in the current version, it could be added to a `_components/`
folder in `docs/layout/`. When the versioning script is run, the component will be copied to `versioned_docs/version-{X}/layout/_components/` and there
will now be a separate component in `docs/layout/_components/` that can be deleted or updated to the latest version. The same concept applies to
images and other files.

If components are meant to be shared across versions, they can be put in `src/components/`. If images and other served files are meant to be shared
across versions they can be put in `static/`.

## Auto Generated Files

All markdown files in these directories are generated from [scripts](../scripts):

- `docs/cli/commands/`
