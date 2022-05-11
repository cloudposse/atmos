# Theme folder

This folder is used to override the base docusaurus theme. It houses swizzled components. Components should NOT be swizzled unless absolutely
necessary to allow for changes in future versions. If it is possible to shallow swizzle a component using the `@theme-original` alias, then that
should be heavily considered. Swizzled components should be added to the prettier ignore and all code updates should be marked with comments to allow
more seamless version updating. The styles file for components that have been unsafely swizzle should absolutely not be edited. All styling should be
done from the [component partials](/src/styles/components).
