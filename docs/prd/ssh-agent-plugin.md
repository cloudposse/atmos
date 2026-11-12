# PRD: SSH identities (`cloudposse/ssh` plugin, and identities on remote reads)

**Status:** Proposed. Backed by a working proof of concept (`~/se-agent-poc`, 2026-11-06); no plugin or core code exists yet. Builds on the [plugin platform](plugins/README.md), its [authentication](plugins/extensions/authentication.md) and [command](plugins/extensions/commands.md) contracts, and [Git operations](git-ops.md).

## Problem and outcome

SSH is how Ansible reaches hosts, how Git signs commits, and how private Git sources are fetched. In each case the key is who you are. Atmos has identities for clouds but none for SSH, so SSH keys are managed outside it: exportable files on disk, keys in environment variables or `sshkey=` URL parameters, or a desktop agent (1Password, Secretive) that refuses to sign while the screen is locked. That last point blocks unattended work, such as a coding agent committing or a playbook running while nobody is at the keyboard.

Outcome, in two parts:

1. **A plugin makes an SSH key an Atmos identity.** Its private key is generated inside the machine's security hardware and cannot be exported. It works while the screen is locked, edits no global configuration, and puts no secret in the environment.
2. **Core lets every remote read name an identity.** Vendored sources, component sources, and remote imports accept `identity:`, as Git repositories, stores, and plugin registries already do.

```shell
atmos auth login --identity ops                       # once per machine: creates and enrolls the key
atmos ansible playbook web -s prod --identity ops     # Ansible logs in to hosts with it
atmos git commit infra -m "..."                       # commits are signed with it
atmos vendor pull                                     # sources that name the identity are fetched with it
atmos auth exec --identity ops -- claude              # a coding agent's own git and ssh use it
```

## Evidence from the proof of concept

| Finding | Result |
|---|---|
| Secure Enclave key with `AfterFirstUnlockThisDeviceOnly` and `.privateKeyUsage` only | Signed 43 of 43 commits with the screen locked, all verified by `git verify-commit` |
| Control key with `WhenUnlockedThisDeviceOnly` | Refused 42 of 43 while locked (`errSecInteractionNotAllowed`, -25308); the one success was inside the grace period right after locking |
| Entitlements, keychain, Developer Program | None needed. An ad-hoc-signed binary run as a per-user LaunchAgent is enough |
| Same key through the C `Security.framework` API (no CryptoKit, no Swift) | Create, reload from blob, and sign all work; blobs created by CryptoKit load unchanged |
| Key blob used by an unrelated binary | **Works.** Any process running as the user that can read the blob can sign |
| 1Password `op-ssh-sign` in global Git config | Intercepts signing unless `gpg.ssh.program` is overridden per process |
| `gh` token on the test machine | Lacks the `ssh_signing_key` scope, so it cannot register a signing key as is |

The fourth row is what makes a plugin possible: official plugins are cross-built on Linux with `CGO_ENABLED=0` (`.github/workflows/plugin-packages.yaml`), so the macOS code must reach the Secure Enclave without cgo or Swift.

## What exists today

Read from the code on this branch.

**Plugin platform**

- **Exports:** auth providers, auth identities, and commands (`pkg/plugin/bootstrap/descriptors.go`). One package can export several. Integrations and stores are follow-ons.
- **Plugin identities need an upstream** (`via` a provider or another identity).
- **Credential types are fixed.** Between a provider and an identity only `secret-values/v1` (a string map), `oidc/v1`, and `aws/v1` can be decoded; an identity can output only `environment/v1` or `aws/v1`. A plugin cannot define its own type.
- **Every use authenticates.** A chain rooted at a plugin provider is never served from a cache: the provider's and the identity's authentication both run on each use, each in a fresh plugin process. Only `atmos auth exec`, `shell`, `env`, and `atmos git` first try the stored identity credential.
- **Credentials must have an expiry.** A missing one is rejected; a far-future one is accepted.
- **A plugin cannot show the user text during authentication.** The host discards progress events, interaction events, and error messages from auth calls. A plugin can only open a browser URL and listen on a loopback port. A device code cannot be displayed.
- **Environment credentials** are a fixed map. Every value is masked in Atmos output, and the plugin cannot see or append to the caller's environment.
- **Private state** is separate for each provider and identity, is not available to commands, and is wiped by the host on logout.
- **Commands** run through `atmos plugin run <alias> -- <args>` and receive the raw arguments. They get HTTP but no stdin, TTY, or state. There is no top-level shortcut yet; the `aliases:` setting provides one.
- **Empty environment.** Plugin processes start with no `HOME` or `PATH`, in the install directory. The home directory is still available from the user database (verified with cgo off).
- **The process group is killed after every call,** so a daemon cannot be a child of a plugin call.
- **Only one of `spec` and `principal` is validated** against the identity's schema.
- **Project scope only.** The release is pinned per project in `.atmos.plugins.lock.yaml`.
- **Release contents:** `plugin.yaml`, `schemas/**`, and one native executable per platform, whose install path changes with every release. No app bundle, helper binary, or provisioning profile.

**Remote reads**

- Everything Atmos fetches itself goes through go-getter in `pkg/downloader`: `atmos vendor pull`, component `source:` provisioning, remote stack imports, remote `atmos.yaml` imports. Its Git getter runs the system `git` with the process environment.
- **None of these accept an identity.** `AtmosVendorSource`, `VendorComponentSource`, `StackImport`, and `ConfigImport` have no such field. Private reads rely on whatever is in the process environment, plus one hard-wired broker (`pkg/auth/broker`) that provisions Atmos Pro's `github/sts` in CI.
- `git.repositories.<name>.auth.identity` is specified to work this way for `atmos git` ([Git operations](git-ops.md)), but `cmd/git/env.go` passes no identity provider, so the setting has no effect today. Git hooks and provisioner targets do pass one.
- `atmos vendor update` and `vendor diff` list remote versions in-process with go-git and no credentials.
- Vendoring fetches sources concurrently, so credentials exported into the process (as the broker does) cannot differ per source.

## Design

### Model: the upstream is who vouches for the key

A hardware key on its own proves nothing. It becomes an identity when a party others already trust says "this key speaks for X". Registering a key on GitHub, an `authorized_keys` entry, an EC2 Instance Connect push, and an SSH certificate are the same act with different authorities and lifetimes. Atmos already has a slot for "who vouches for this identity": the `via` link.

| Upstream (`via`) | What `atmos auth login --identity <name>` does | Trust lasts | Phase |
|---|---|---|---|
| `ssh/self` provider | Creates the key if missing. You distribute the public key yourself | Until removed | 1 |
| A provider that yields a GitHub token (`secrets/store` from `cloudposse/auth-secrets`) | Also makes sure the key is on that GitHub account for signing and, optionally, authentication | Until removed | 1 |
| `github/user` provider (device login) | The same, without a stored token | Until removed | Blocked: core cannot show a device code |
| An AWS identity | Pushes the public key to the instance (EC2 Instance Connect) | About a minute | 4 |
| A Vault or CA identity | Obtains a certificate for the key | Minutes to hours | 4 |

So there is no "register" command. Enrollment is the identity's login step and it is idempotent. Each row is an implementation of one [authority interface](#authorities-the-enrollment-interface); [Managing keys on GitHub](#managing-keys-on-github) shows the user's side.

```yaml
plugins:
  packages:
    ssh:
      source: oci://ghcr.io/cloudposse/atmos/plugins/ssh
      version: "^0.1.0"
    secrets:
      source: oci://ghcr.io/cloudposse/atmos/plugins/auth-secrets
      version: "^1.1.0"
aliases:
  ssh: plugin run ssh --
stores:
  keychain:
    type: keychain
    secret: true
auth:
  providers:
    github-token:
      kind: secrets/store               # a GitHub token with the key-management scopes
      spec:
        credentials:
          token: {store: keychain, path: github/ssh-keys}
  identities:
    ops:
      kind: ssh/key
      via: {provider: github-token}
      spec:
        authority: github               # who vouches for the key; `self` when nobody does
        store: secure-enclave           # secure-enclave | tpm | file
        allow: [signing, authentication]
        require: unlocked               # omit to let the key be used while the screen is locked
        user: deploy                    # SSH login name on the remote side, optional
git:
  repositories:
    infra:
      auth: {identity: ops}
      commit: {signing: always}
```

The configuration is shared; the key is not. Each person's login creates and enrolls that machine's own key.

| Field | Meaning |
|---|---|
| `spec.authority` | Which [authority](#authorities-the-enrollment-interface) vouches for the key. It reads its credentials from the upstream's string map; the token is the `token` entry |
| `spec.store` | Where the key is held |
| `spec.allow` | What the key may do: `signing`, `authentication`, or both. It limits the identity's agent socket and decides what is enrolled with the authority |
| `spec.require` | What must hold for the key to be used. See [Key stores](#key-stores) |
| `spec.user` | The SSH login name on the remote side. It is in `spec` because the host validates only one of `spec` and `principal` |
| `spec.rewrite_https` | Hosts whose HTTPS Git URLs are rewritten to SSH |

Each identity has its own key, named after the identity. `spec.key` can name a shared key instead; identities that share one must agree on `store` and `require`, or validation fails.

**Authentication runs on every use, so it must be cheap.** When a receipt on disk already matches the key and the `allow` list, the identity makes no network call and needs no upstream interaction; it works offline and while locked. It contacts the authority only when something changed: a new key, a changed `allow` list, or a deleted key to revoke.

### What the identity provides

An `environment/v1` credential:

| Variable | When | Purpose |
|---|---|---|
| `SSH_AUTH_SOCK` | always | The identity's own agent socket |
| `ANSIBLE_REMOTE_USER` | `spec.user` set and `authentication` in `allow` | Ansible's login name |
| `GIT_CONFIG_GLOBAL` | `signing` in `allow`, or `spec.rewrite_https` set | A generated file that first includes the user's own global Git configuration, then sets `gpg.format=ssh`, `gpg.ssh.program`, `user.signingkey=key::<public key>`, `commit.gpgsign=true`, `tag.gpgsign=true`, and any `url."ssh://git@<host>/".insteadOf=https://<host>/` rewrites |

`GIT_CONFIG_COUNT` is not used: the plugin cannot append to entries the caller already has, the host would overwrite them, and short values such as `ssh` would be masked everywhere in Atmos output. Nothing in this environment is secret, but the host masks every value, so these paths appear masked in Atmos output.

### Consumers

| Consumer | How it names the identity | Needs |
|---|---|---|
| Terraform, Packer, Helmfile components | `--identity ops` or component auth | Nothing new. The environment reaches the tool, so `terraform init` module sources and provisioners use the key |
| Ansible components | `--identity ops` | Core fix on this branch: the identity's environment was not passed to `ansible-playbook`. Ansible still authenticates only on an explicit `--identity` |
| `atmos git` | `git.repositories.<name>.auth.identity` | Core fix on this branch: `atmos git` ignored the configured identity |
| Anything else | `atmos auth exec`, `shell`, `env` | Nothing new |
| `atmos vendor pull`, component `source:`, remote stack imports | `identity:` on the source or import | **Core change below** |

### Core change: identities on remote reads

Add `identity:` wherever a remote location is declared, using the flat field that commands, stores, MCP servers, secrets, workflows, and plugin registries already use:

```yaml
# vendor.yaml
spec:
  sources:
    - component: vpc
      source: git::ssh://git@github.com/acme/modules.git//vpc?ref={{.Version}}
      version: 1.4.0
      identity: ops

# stack manifest
import:
  - path: git::ssh://git@github.com/acme/stacks.git//catalog/vpc?ref=v2
    identity: ops
components:
  terraform:
    vpc:
      source:
        uri: git::ssh://git@github.com/acme/modules.git//vpc
        version: 1.4.0
        identity: ops
```

- **Fields:** `AtmosVendorSource.Identity`, `VendorComponentSource.Identity`, and `StackImport.Identity`, plus the JSON schemas in `pkg/datafetcher/schema/`. `ConfigImport` is left out; see the limits.
- **Defaults:** `vendor.identity` and `imports.identity` in `atmos.yaml`, so a project names its source identity once. A default applies to every remote go-getter source, not only Git ones.
- **Precedence:** `--identity` flag, then the field on the entry, then the section default, then the existing ambient broker, then the process environment. The flag applies to `atmos vendor pull` only. On component commands it selects the component's own identity, and it is not reachable during stack processing, so sources and imports there use the field and the default.
- **One seam.** The downloader gains a per-fetch environment (`downloader.WithEnvironment(ctx, env)`), composed with the existing `git.ComposeEnvironment` and `EnsureIdentityEnvironment`. The Git getter uses it in place of the bare process environment. Nothing is exported with `os.Setenv`, so two sources fetched concurrently can use different identities.
- **Not SSH-specific.** Any identity works: an AWS identity for an S3 source, `oci/user` for an OCI source, a `github/sts`-linked identity for HTTPS. SSH identities are one beneficiary.
- **Same file, laptop and CI.** The identity name is stable; its kind can differ by profile. `source-reader` is an `ssh/key` on a workstation and an Atmos Pro identity with `github/sts` in CI, and `vendor.yaml` does not change.
- **Limits:**
  - Module sources inside `.tf` files cannot name an identity; they use the component's.
  - A remote `import:` in `atmos.yaml` cannot name an identity. When imports resolve, only the importing file's own `auth:` section is loaded, so an identity defined in another fragment or profile is not visible. Supporting it means reworking configuration loading.
  - `atmos vendor update` and `vendor diff` do not use the identity when listing versions; only the fetch does.
  - Only Git subprocesses receive the identity's environment. S3 and OCI sources authenticate as before.
  - Outside CI, a named identity with no valid session fails with a hint to run `atmos auth login`; it never prompts mid-fetch. In CI the identity authenticates directly, because providers such as OIDC have nothing stored.
- **Replaces:** the `sshkey=` go-getter URL parameter, which puts a private key in configuration.

### Plugin package

`plugins/ssh/`, published as `oci://ghcr.io/cloudposse/atmos/plugins/ssh`, name `cloudposse/ssh`, starting at `0.1.0`. Its own Go module, added to `plugins/releases.yaml`. One executable with two roles:

1. **Plugin server**, started by Atmos: the auth exports and the command tree.
2. **Agent daemon**, started by launchd or systemd and never by Atmos: invoked directly as `plugin agent-serve --config <file>`, it skips the plugin handshake and serves sockets. The service definition points at a stable copy of the executable, because the installed path changes with every release.

Exports:

| Export | Kind | Role |
|---|---|---|
| Provider | `ssh/self` | No authority; the key is self-asserted |
| Provider | `github/user` | Deferred. Device login to a GitHub user account; needs core to display a device code |
| Identity | `ssh/key` | The key, its enrollment with the upstream, and its environment |
| Command | `ssh` | Key and agent management |

All six platforms ship from the first release so a mixed-OS team can activate the package everywhere. Hardware key stores report "not supported on this platform" where they don't apply.

### Authorities: the enrollment interface

An **authority** is a party that can vouch for a public key. The `ssh/key` identity never talks to GitHub directly; it hands the key to the authority named in `spec.authority`. GitHub is the first implementation.

```go
// Authority vouches for a public key using an upstream credential.
type Authority interface {
    // Name is the registry key, such as "github".
    Name() string
    // Supports lists the uses it can vouch for.
    Supports() []Use // UseSigning, UseAuthentication
    // Ensure makes the authority trust the key for the requested uses. It is idempotent:
    // a key that is already trusted returns the existing receipt and changes nothing.
    Ensure(ctx context.Context, credentials map[string]string, request Request) (Receipt, error)
    // Revoke withdraws an enrollment. It is idempotent: a missing enrollment is success.
    Revoke(ctx context.Context, credentials map[string]string, receipt Receipt) error
}

type Request struct {
    Key        ssh.PublicKey
    Uses       []Use    // from the identity's `allow`
    Title      string   // "<key>@<hostname> (Atmos)"
    Principals []string // remote usernames, for authorities that issue certificates
    Target     string   // host or instance, for authorities that enroll per host
}

type Receipt struct {
    Authority   string           // "github"
    Subject     string           // the account, role, or host that now trusts the key
    References  map[Use]string   // the authority's own IDs, needed to revoke
    Certificate []byte           // set by authorities that issue certificates
    ExpiresAt   time.Time        // zero means trusted until revoked
}
```

- **Registry.** Authorities register by name (`authority.Register`). The platform has one usable credential type between a provider and an identity, a string map, so the authority cannot be chosen by credential type. The identity fails before any network call when the named authority is unknown or a requested use is unsupported.
- **Receipts** are JSON files on disk beside the key, one per key and authority. They hold no secret. They cannot live in the plugin's private state, which the host wipes on logout.
- **One contract for long-lived and short-lived trust.** A receipt with no expiry is an enrollment (GitHub). A receipt with an expiry and a certificate is an issuance (Vault). The identity's credential expires when its receipt does.

The auth lifecycle maps onto the interface, so no authority needs its own commands:

| Atmos action | What the identity does |
|---|---|
| `atmos auth login --identity <name>`, and every other use | Creates the key if missing. If a receipt matches the key and its `allow` list, nothing else. Otherwise `Ensure`, then `Revoke` for this machine's receipts whose key no longer exists. Makes sure the agent is running |
| `atmos auth logout <name>` | `Revoke` for this machine's receipts whose key no longer exists, using the token the identity saved in its own private state when it last authenticated. A live key stays enrolled |

A short-lived receipt (a certificate) is renewed by the same step when it has expired, which works unattended only if the upstream can authenticate without interaction.

The rule for removal is: **an enrollment is revoked once its key is gone.** Logging out alone never removes a key from the authority, so a routine `atmos auth logout --all` cannot strip a working machine's keys.

**Adding an authority**

1. Implement `Authority` in `plugins/ssh/authority/<name>/` and register it.
2. If no existing Atmos provider or identity yields a credential it can use, add a provider kind to the package (`gitlab/user`, for example).
3. Pass the shared conformance suite, which runs every registered authority against a fake server: `Ensure` twice changes nothing, each use is enrolled separately, `Revoke` twice succeeds, a credential without the needed permission produces an error naming it, and receipts contain no secret.
4. Add a row to the table below and to the docs.

The identity, the agent, the configuration schema, and the commands do not change. A test-only fake authority proves the identity has no GitHub-specific code.

| Authority | Upstream credential | Uses | Receipt | Phase |
|---|---|---|---|---|
| `github` | Any provider that yields a `token`: `secrets/store` today, `github/user` later | signing, authentication | No expiry | 1 |
| `gitlab` | Any provider that yields a `token` | signing, authentication | No expiry | later |
| `ec2-instance-connect` | A built-in AWS identity | authentication | About a minute, per instance | 4 |
| `vault-ssh` | A Vault identity | authentication | Certificate, minutes to hours | 4 |

Limit: authorities are compiled into this package. Another plugin can supply the upstream login, but not a new authority, until plugins can call each other.

### Managing keys on GitHub

The identity is configured once, in the shared `atmos.yaml` shown above. Everything a user does afterwards is one of these:

| I want to | Run | What happens on GitHub |
|---|---|---|
| Set up this machine | `atmos auth login --identity ops` | A new signing key titled `ops@<hostname> (Atmos)` |
| See what this machine has enrolled, and the last error | `atmos ssh key status` | Nothing; reads local receipts |
| Also push over SSH | Add `authentication` to `allow`, then `atmos auth login --identity ops` | The same key is added as an authentication key |
| Set up a second machine | `atmos auth login --identity ops` there | A second key; each machine has its own |
| Rotate this machine's key | `atmos ssh key delete ops --yes`, then `atmos auth login --identity ops` | The new key is added and the old one removed, in one login |
| Retire this machine | `atmos ssh key delete ops --yes`, then `atmos auth logout ops` | This machine's key is removed |
| Deal with a lost machine | Delete `ops@<hostname> (Atmos)` under GitHub Settings, SSH and GPG keys | Removed by hand; the lost machine cannot be asked to do it |

First login on a machine. The host shows no text from a plugin during authentication, so the detail comes from `key status`, which also reports the last failure:

```console
$ atmos auth login --identity ops
$ atmos ssh key status
KEY   STORE            REQUIRE    AUTHORITY   SUBJECT    ALLOW     SINCE        EXPIRES   LAST RESULT
ops   secure-enclave   unlocked   github      osterman   signing   2026-11-12   never     enrolled
```

GitHub specifics, all inside the `github` authority:

- **API:** `POST /user/ssh_signing_keys` for signing and `POST /user/keys` for authentication; the matching `DELETE` calls to revoke. A key GitHub reports as already present on the account is adopted, not duplicated.
- **Token:** a GitHub token stored in any Atmos secret store and read through `secrets/store`. It needs `write:ssh_signing_key`, plus `write:public_key` when `allow` includes `authentication`. A token lacking a scope produces an error naming it, visible in `key status`.
- **Device login (`github/user`) is deferred.** It needs two things that do not exist: a client ID that Atmos owns (embedding the GitHub CLI's is not an option, because the consent screen and audit log would name GitHub CLI for a request Atmos made), and a way for a plugin to show the user a code.
- **Without a token:** the identity uses `authority: self` with the `ssh/self` provider, and `key show --format github` prints the public key, the settings URL, and the equivalent `gh ssh-key add --type signing` command.
- **Bots:** the endpoints are per user. A machine-user account can hold a signing key; a GitHub App cannot.

### Key stores

Each returns a `crypto.Signer` plus an OpenSSH public key:

| Store | Platforms | Phase | Notes |
|---|---|---|---|
| `secure-enclave` | darwin | 1 | `Security.framework` through `ebitengine/purego`. P-256. With no `require` the key is created as `AfterFirstUnlockThisDeviceOnly`; with `require: unlocked` as `WhenUnlockedThisDeviceOnly`. Stored as a wrapped blob, mode 0600, under the Atmos XDG data directory |
| `file` | all | 1 | Ordinary OpenSSH private key file. For tests and platforms without hardware support. Documented as not hardware-protected |
| `tpm` | linux, windows | 3 | `google/go-tpm` (pure Go). Needs `/dev/tpmrm0` access on Linux |

Existing keys cannot be imported into the Secure Enclave; each machine generates its own.

**`require`** lists what must hold for the key to be used. It takes one value or a list, and every term is enforced by the key store.

| `require` | Screen locked | After a reboot, before first login |
|---|---|---|
| omitted | Usable | Refused |
| `unlocked` | Refused | Refused |

- **The key store enforces it, not the plugin.** The agent contains no lock-state check that grants or refuses signing. In the proof of concept a separate process that loaded the key blob directly, bypassing the agent, was refused by the enclave while locked. So `require: unlocked` holds even against other processes running as the user.
- **It is fixed when the key is created** and cannot be changed afterwards. When the configured value differs from the existing key's, login fails and says to rotate the key (`key delete`, then log in). It never keeps the old setting silently.
- **A store that cannot enforce a term rejects it** at validation instead of accepting a promise it cannot keep. `tpm` and `file` reject `unlocked`.
- **Omitting it is not "always".** The key still does not work after a reboot until the first login.
- `key status` shows what each key was created with.
- **Later terms** must also be hardware-enforced. `presence` (Touch ID per signature) is the candidate; it is deferred because it has not been tested from a LaunchAgent.

Why this word:

- **Not `when`.** A `when` on imports, tasks, and workflow steps is a condition Atmos evaluates at run time and that can be edited freely. This is a constraint the hardware enforces and that is fixed at creation. Conditions Atmos evaluates, such as `local`, are out of scope; they would be a top-level `when` on identities, a separate core change.
- **Same meaning as `version.require`:** a constraint that must hold or the operation fails.
- **Not `policy`,** which means admission rules in Atmos (`policy.d/`, `plugins.policy`).
- **Not the same as `required: true`** at the top level of an identity, which asks Atmos to authenticate it automatically.

### Agent

`golang.org/x/crypto/ssh/agent` over Unix sockets (a named pipe on Windows in phase 3).

- **One socket per identity.** The socket exposes only that identity's key and signs only what `allow` permits. A Git signature request is an SSHSIG blob that begins with the magic bytes `SSHSIG` and carries the namespace `git`; an SSH login request does not. A signing-only socket therefore cannot be used to log in to a server, and an authentication-only socket cannot sign commits.
- **Peer check.** Connections from another uid are rejected.
- **Audit log.** One line per request: time, identity, request type, result, screen lock state.
- **Lifecycle.** `agent start` writes a LaunchAgent plist (systemd user unit on Linux) pointing at the installed executable and loads it with `/bin/launchctl bootstrap gui/<uid>`. The daemon is launchd's child, in the user's login session, and survives Atmos exiting. Using an identity starts it when it is not running.

### Commands

Invoked as `atmos plugin run ssh -- <args>`, or `atmos ssh <args>` with the alias above:

```
key list
key show <name> [--format authorized-key|allowed-signers|github|json]
key status [<name>]          # this machine's keys and receipts: store, require, authority, subject, allow, expiry
key delete <name> --yes      # local only; the authority is updated at the next login or logout
agent start | stop | status | log
test [--soak <duration>]     # sign and verify in a scratch repo; reports lock state
```

Keys are created by `atmos auth login`. Destructive commands need `--yes` because commands cannot prompt. Commands never call an authority: they have no access to auth credentials, so every remote change goes through login or logout.

## Security model

- **Protected:** the key cannot leave the machine. A stolen disk, backup, or copied blob is useless elsewhere. After a reboot nothing signs until the first login.
- **Not protected:** other processes running as the same user. They can read the blob and sign directly, bypassing the socket's `allow` limits. Those limits are a real boundary only for callers that cannot read the key directory: containers and sandboxes given just the socket.
- **Documented rule:** never mount the key directory into a sandbox.
- **Closing the gap** needs keychain-held keys restricted to a signed app, which needs an Apple Developer Program membership, a provisioning profile, and an app bundle. The plugin release format cannot carry those today. Deferred.
- **Scoped use.** A source or import uses a key only when it names the identity; nothing is exported process-wide.
- **Authority token.** The GitHub token should carry only key-management scopes. It lives in the secret store the user chose. The identity keeps a copy in its own private state so logout can revoke enrollments; the host removes that copy at logout.
- The daemon is started by launchd directly, so Atmos's re-verification of cached plugin files does not run before it starts. Accepted for now, given the same-user boundary above.

## Delivery

The plugin and the core change are independent and can land in either order. One pull request each.

**Phase 0: spikes**

| # | Question | Result |
|---|---|---|
| 1 | Can Go with `purego` and cgo off create, reload, and sign with a Secure Enclave key, and load the POC's blob? | **Yes.** Verified on hardware; builds for all six platforms |
| 2 | Does that binary sign while the screen is locked, as a LaunchAgent, for a Git signature and an SSH login? | Not run yet. Needs a person to lock the screen |
| 3 | Can a plugin run `/bin/launchctl` and find the home directory with an empty environment? | **Yes.** `os/user` returns it with cgo off; `launchctl` runs by absolute path |
| 4 | Can a plugin identity return a credential that does not expire? | **No.** An expiry is required; a far-future one is accepted |
| 5 | Does `atmos auth login` re-run the identity's authentication? | **Yes, and so does every other use.** Authentication must be cheap and idempotent |
| 6 | Can logout reach the authority? | **Yes,** if the identity saved the token in its own private state when it authenticated |
| 7 | Does an Ansible run with `--identity` receive the identity's environment? | **It did not; fixed.** A test reproduced the missing environment, and the executor now passes it the way Helmfile and Packer do |
| 8 | Does a stored `environment/v1` credential work in a fresh process (`atmos auth exec`, `atmos git`)? | **It did not; fixed.** A two-manager test against the real `auth-secrets` plugin failed on all three passive paths. The manager now hands the stored credential back to the identity |
| 9 | Does `atmos git` use `git.repositories.<name>.auth.identity`? | **It did not; fixed.** `cmd/git` passed no identity provider. It now resolves the flag, then the repository's identity |

Rows 4 to 6 come from reading the platform code, not from running it. Row 2 is the one acceptance step still open.

**Phase 1a: plugin, macOS MVP (`0.1.0`)**
- Package scaffold from `atmos plugin create`, manifest, schemas, `releases.yaml` entry.
- `ssh/self` provider, `ssh/key` identity, `secure-enclave` and `file` stores, daemon with per-identity sockets and audit log, commands. GitHub enrollment uses a token from `secrets/store`; the `github/user` device login waits on core.
- The `Authority` interface, its registry, receipts, the `github` authority, and the conformance suite.
- Tests on Linux CI against the `file` store: agent protocol, SSHSIG parsing, `allow` limits (allowed and refused requests), environment composition, the conformance suite against a fake GitHub API, and the login and logout reconciliation rules against a fake authority. `plugin.tests.yaml` fixtures for the auth exports.
- Manual acceptance on a real Mac: the POC's soak test, ported to `test --soak`. GitHub's macOS runners are virtual machines without a Secure Enclave, so this cannot run in CI.
- Docs: package README, website page, blog post, roadmap entry.

**Phase 1b: core**
- Identities on remote reads: the three fields, section defaults, schemas, precedence, and the downloader's per-fetch environment.
- Tests with a mock identity environment provider: per-source environment reaches the Git subprocess, two concurrent sources with different identities stay isolated, precedence, and the no-identity path is unchanged.
- Make `atmos git` use the configured identity.
- Pass an identity's environment to `ansible-playbook`.
- Make a stored `environment/v1` credential usable in a fresh process, if the reproducing test confirms the defect.
- Docs for vendoring, `source:`, and imports.

**Phase 1c: core, what a plugin can tell the user** (needs a decision; see open questions)
- Display a provider's interaction event, so a device code can be shown.
- Surface a bounded, masked error message from auth calls.
- Let a plugin mark environment values as not sensitive.

**Phase 2: sandboxes**
- Mount an identity's socket into devcontainers and containers.
- Spike first: Docker Desktop on macOS does not reliably pass arbitrary host Unix sockets into containers.

**Phase 3: Linux and Windows**
- `tpm` key store, systemd user unit, Windows named pipe and service.

**Phase 4: more authorities**
- `ssh/key` chained `via` an AWS identity (EC2 Instance Connect) or a Vault or CA identity (certificates), so hosts trust an authority instead of each machine.
- Host restrictions per identity, using the destination information newer OpenSSH sends to agents.

**Deferred**
- Top-level `atmos ssh` without an alias (waits on plugin command shortcuts in core).
- Touch ID keys; keychain-restricted keys.
- Secure Enclave as a secrets store or SOPS age recipient (waits on the stores extension).

## Acceptance criteria

**Plugin (phase 1a)**
- `atmos auth login --identity ops` on a fresh Mac creates the key and enrolls it on GitHub; a second run changes nothing.
- Every row of the [Managing keys on GitHub](#managing-keys-on-github) table works as described: adding a use, a second machine, rotation, and retiring a machine.
- `atmos auth logout` with a live key leaves it enrolled.
- A second authority can be added without changing the identity, the agent, the configuration schema, or the commands. A test-only fake authority demonstrates this and passes the conformance suite.
- With the screen locked and no prompt: `atmos git commit` produces a commit `git verify-commit` accepts, even with 1Password's signer configured globally; and `atmos ansible playbook --identity ops` connects to a test host.
- A `require: unlocked` key is refused while locked, and `key status` and the audit log name the cause.
- Changing `require` for an existing key makes login fail with instructions to rotate; `require: unlocked` on the `file` store and any unknown term fail validation.
- A signing-only identity cannot authenticate an SSH login; an authentication-only identity cannot sign a commit.
- `agent status` reports when the running daemon is a different version from the installed plugin.
- On Linux and Windows the package installs and activates; `secure-enclave` fails with a clear "not supported" error; the `file` store works.
- The release is built on Linux with cgo disabled and passes `atmos plugin release --check`.

**Core (phase 1b)**
- A vendored source, a component `source:`, and a remote stack import that each name an identity are fetched with that identity's environment and nothing is written to the process environment.
- Two sources with different identities in one `atmos vendor pull` each use their own.
- Sources without an identity behave exactly as before, including the Atmos Pro broker in CI.
- An identity that needs interactive login fails with the `atmos auth login` hint.

## Open questions

1. **What a plugin can tell the user.** The host discards plugin text during authentication, deliberately, because it could contain secrets. That blocks the device login, makes failures generic, and masks harmless values. Phase 1c lists the three core changes that would lift it. Until then enrollment needs a stored token, and diagnostics live in `atmos ssh key status`.
2. **GitHub app.** A device login also needs a client ID that Atmos owns: register an OAuth app, or check whether the existing Atmos Pro GitHub App can be granted the signing-keys account permission.
3. **Two identities in one run.** A run uses one identity's environment. A playbook that needs cloud credentials for dynamic inventory as well as the SSH identity has to chain them, which depends on question 5.
4. **Signing with a different push identity.** A repository has one `auth.identity`. If pushes use a `github/sts` App token on another identity, the SSH identity cannot also be named there. A core `git/signing` integration, or a separate `commit.identity` field, would solve it.
5. **Chaining from built-in identities.** A plugin identity can accept AWS credentials as its upstream (`aws/v1`). No other built-in credential family can be passed to a plugin today, so a Vault upstream needs core work.
6. **Old commits after a key is removed.** Whether GitHub keeps showing earlier commits as Verified once their signing key is deleted needs confirming; it decides how freely rotation should be recommended.
7. **Removing a lost machine's key from another machine.** Today that is done by hand on GitHub. An authority could list and revoke enrollments it did not create, at the cost of a wider interface.
8. **Per-user daemon, per-project plugin.** Two projects can lock different plugin versions while sharing one daemon and key directory. Proposal: the newest started daemon wins and `agent status` reports the mismatch.
9. **Explicit `require`.** Omitting it lets the key be used while locked, which is the purpose of this package. The cautious alternative is to make the field mandatory on `ssh/key` identities, with an empty list meaning no requirement, so locked-screen use is always a written choice.
10. **Developer Program.** Join it to close the same-user gap, and extend the plugin release format to carry a signed macOS helper?
