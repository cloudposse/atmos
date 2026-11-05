# Native auth plugin recording fixture

The recording creates the Go OIDC starter, runs its generated `atmos build`, `test`, and `push` commands using the local SDK, publishes
its native package to an ephemeral Atmos OCI registry emulator,
lists discovery sources in table and JSON formats, searches for the release, installs it,
inspects its details, and executes the installed command. After the emulator stops,
search runs again against the cached discovery index. The same local OIDC fixture
then verifies the downloaded executable: PKCE login, token validation, credential
exchange, cached reuse, refresh, and logout. No external issuer or cloud credentials
are used, and no artifacts are pushed outside localhost.

Regenerate from the repository root:

```shell
atmos --chdir=demo/casts casts generate examples native-auth-plugins lifecycle
atmos --chdir=demo/casts casts validate examples native-auth-plugins lifecycle
```

Requires Go, Python 3, and a running supported container runtime. Local port 15556
must be available. Recording setup changes the generated project's default port from 15557 to 15556. Its `acme-oidc-registry` component does not reuse a manually started `plugin-registry` on port 15555. The registry is unsigned for this local demonstration; publisher signing requires a
real signing identity and is covered separately in the plugin documentation.
The recording uses a disposable `.context/cast-fixtures` copy
with its own plugin cache; its emulator is stopped even if a recording step fails.
The `demo test-downloaded` command copies the installed package into a test directory
and adds the generated fixture declaration, because release archives deliberately
contain only runtime files. It executes the downloaded binary without rebuilding it.
