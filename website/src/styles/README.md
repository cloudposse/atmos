# Styles folder

This folder houses a global style file that is brought into the theme in [docusaurus.config.js](/docusaurus.config.js).

In addition, there is a components folder that is used to style theme components instead of swizzling them.

Related issue: https://github.com/facebook/docusaurus/pull/5987

Since theme styles cannot be overridden at the moment, the base #\_\_docusaurus tag is being used to +1 the specificity of selectors.
