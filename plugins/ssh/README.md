# ssh

Make an SSH key an Atmos identity. The key is generated inside the machine's security hardware where there is any, cannot be exported, works while the screen is locked (unless you say otherwise), edits no global configuration, and puts no secret in the environment. The package exports the `ssh/self` provider, the `ssh/key` identity, and the `ssh` command, and is published at:

```text
oci://ghcr.io/cloudposse/atmos/plugins/ssh
```

SSH is how Ansible reaches hosts, how Git signs commits, and how private Git sources are fetched. With this package each of them uses a key that Atmos knows by name:

```shell
atmos auth login --identity ops                       # once per machine: creates and enrolls the key
atmos auth exec --identity ops -- git commit -m "..."  # the commit is signed with it
atmos auth exec --identity ops -- claude              # a coding agent's own git and ssh use it
```

## Configuration

A key proves nothing on its own. It becomes an identity when a party others already trust says "this key speaks for X". That party is the identity's `authority`, and the identity's upstream (`via`) supplies the credential the authority needs.

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
        store: secure-enclave           # secure-enclave | file
        allow: [signing, authentication]
        require: unlocked               # omit to let the key be used while the screen is locked
        user: deploy                    # SSH login name on the remote side, optional
git:
  repositories:
    infra:
      auth: {identity: ops}
      commit: {signing: always}
```

Without a token, use the `ssh/self` provider and `authority: self`. You then give the public key to whoever needs it (`key show --format github` prints what GitHub wants):

```yaml
auth:
  providers:
    local:
      kind: ssh/self
  identities:
    ops:
      kind: ssh/key
      via: {provider: local}
      spec:
        store: secure-enclave
        allow: [signing]
```

| Field | Meaning |
|---|---|
| `authority` | `self` (default) or `github`. The token is read from the upstream credential's `token` entry. |
| `store` | `secure-enclave` (macOS hardware) or `file` (an ordinary key file, not hardware-protected). Required. |
| `allow` | `signing`, `authentication`, or both. Limits the identity's agent socket and decides what is enrolled. Required. |
| `require` | One term or a list. The only term is `unlocked`: the key is refused while the screen is locked. The key store enforces it, it is fixed when the key is created, and a store that cannot enforce it rejects it. |
| `user` | The SSH login name on the remote side. Set as `ANSIBLE_REMOTE_USER` when `authentication` is allowed. |
| `key` | The key's name in the store. Defaults to the identity's name. |
| `rewrite_https` | Hosts whose HTTPS Git URLs are rewritten to SSH. |
| `root` | The plugin's data directory, when it is not the default. The plugin cannot see Atmos's environment, so a relocated Atmos data directory must be named here, and in the command's `config` (below). |

The configuration is shared; the key is not. Each person's login creates and enrolls that machine's own key.

## What the identity sets

| Variable | When |
|---|---|
| `SSH_AUTH_SOCK` | Always. The identity's own agent socket. |
| `GIT_CONFIG_GLOBAL` | `signing` is allowed, or `rewrite_https` is set. A generated file that first includes your own `~/.gitconfig` and `~/.config/git/config`, then sets SSH signing with the identity's key and the URL rewrites. |
| `ANSIBLE_REMOTE_USER` | `user` is set and `authentication` is allowed. |

Every use of the identity authenticates again, so the work is idempotent. When a receipt on disk already matches the key and the `allow` list, there is no network call and no interaction.

## The ssh command

Run `atmos plugin run ssh -- <arguments>`, or `atmos ssh <arguments>` with the alias above.

```text
key list
key show <name> [--format authorized-key|allowed-signers|github|json]
key status [<name>]
key delete <name> --yes
agent start | stop | status | log [--lines N]
test <identity> [--soak <duration>] [--interval <duration>]
help
```

| I want to | Run |
|---|---|
| Set up this machine | `atmos auth login --identity ops` |
| See what this machine has enrolled, and the last error | `atmos ssh key status` |
| Also push over SSH | Add `authentication` to `allow`, then `atmos auth login --identity ops` |
| Rotate this machine's key | `atmos ssh key delete ops --yes`, then `atmos auth login --identity ops` |
| Retire this machine | `atmos ssh key delete ops --yes`, then `atmos auth logout ops` |
| Check that signing works with the screen locked | `atmos ssh test ops --soak 10m --interval 30s`, then lock the screen |

Commands never contact an authority and never prompt: they have no credentials and no terminal. Every change at an authority goes through `atmos auth login` or `atmos auth logout`, and destructive commands need `--yes`. Pass `--root <dir>` to any command, or set `root` under `plugins.packages.<alias>.config`, when the data directory is not the default. `key show` and `key delete` accept `--store <kind>` to pick a store when a key name exists in more than one.

`test` signs a Git payload through the identity's socket and verifies it. With `--soak` it repeats every `--interval` for that long and prints one timestamped line per attempt: `ok`, `refused`, `locked` (the key store refused because the screen is locked), or `unreachable`. It exits non-zero if any attempt did not succeed.

## Security model

- The key cannot leave the machine. A stolen disk, backup, or copied key blob is useless elsewhere. After a reboot nothing signs until the first login.
- The agent runs as a per-user LaunchAgent (macOS) or systemd user unit (Linux), never as a child of Atmos, and rejects connections from other users. Each identity has its own socket, which signs only what `allow` permits: a Git signature on a signing-only socket, an SSH login on an authentication-only socket.
- `require: unlocked` is enforced by the key store, so it holds even against other processes running as you. Without it, the key is usable while the screen is locked, which is what unattended work needs.
- Other processes running as the same user can read the key blob and sign directly, bypassing the socket's `allow` limits. Never mount the key directory into a sandbox.
- The GitHub token should carry only key-management scopes (`write:ssh_signing_key`, plus `write:public_key` for `authentication`). The identity keeps a copy in its private state so logout can revoke enrollments; the host removes it at logout. An enrollment is revoked once its key is gone, so logging out alone never strips a working machine's key.

## Current limits

- No device login. Enrollment needs a token from a secret store (`authority: github`) or you add the public key yourself (`authority: self`). The `github/user` provider waits on core.
- Atmos discards plugin error text during authentication, so a failed login is a generic failure. The reason is in `atmos ssh key status`.
- Hardware keys are macOS Secure Enclave only. The `file` store works everywhere and is not hardware-protected.
- The agent daemon does not run on Windows yet. There the package installs and the `file` store can be listed, but an `ssh/key` identity fails with a "not supported" error.
- Atmos components do not all receive an identity's environment yet; Ansible needs a core change.
- Different projects can pin different plugin versions while sharing one daemon and key directory. `agent status` reports a daemon whose version differs from the installed one, and `agent start` replaces it.

See [release infrastructure](../README.md) for independent versions, public package setup, and the six-platform release matrix.
