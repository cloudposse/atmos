# Helm Plugins

Helmfile requires the Helm Diff CLI plugin for operations such as `diff` and `apply`. Declare it
under the Helmfile component's `plugins` key, alongside `vars`, rather than adding an installation
script or wrapper action:

```yaml
components:
  helmfile:
    nginx-ingress:
      plugins:
        - diff@v3.15.10
```

`diff` is a built-in repository alias. Choose the pin required by the project; other plugins can use
their repository URL and version. Share declarations through component inheritance or top-level
`helmfile.plugins` in a stack manifest (including imported defaults). Plugin lists are resolved
in increasing precedence: type defaults, inherited base components, then the component itself.
They follow `settings.list_merge_strategy` (default: `replace`); `plugins: []` clears inherited
plugins with the default strategy. Declare the `helm` and `helmfile` binaries through
`dependencies.tools`; see
[atmos-toolchain](../../atmos-toolchain/SKILL.md) for tool version configuration.

Normal `atmos helmfile` execution ensures the declared plugins and sets `HELM_PLUGINS` for the child
process. Explicit installation is useful for cache warming or troubleshooting:

```bash
atmos helm plugin install --component nginx-ingress --stack ue2-dev
atmos helm plugin list
```

For CI reuse, include the managed plugin directory in the existing Atmos `ci.cache` configuration.
It lives under the Atmos toolchain root by default; check `toolchain.install_path` when the project
overrides that root. Include plugin declarations in the cache key and separate caches by platform.
Warm the cache with the component-based command above, then restore it into each consumer job's
own directory. See [atmos-cache](../../atmos-cache/SKILL.md) for native cache configuration.

Installation retries, partial-install cleanup, and completed-install checks belong in the existing
`pkg/helm/plugin` installer. Use stack configuration and direct Atmos commands instead of shell
loops, custom downloaders, or plugin setup actions. Helm remains responsible for plugin hooks;
the installer does not infer plugin-specific executable or version commands.

Native `components.helm` uses the embedded Helm Diff Go library and needs no CLI plugin. See
[atmos-helm](../../atmos-helm/SKILL.md) when choosing between the two component types.
