# TryExample

Use this component in changelog posts and command documentation with an associated
runnable example. Add a `## Try it out` section before closing feedback or related
documentation links. In changelog posts, keep it below the `<!--truncate-->`
marker. Preserve existing explanations and recordings.

````mdx
import TryExample from '@site/src/components/TryExample';

## Try it out

<TryExample example="cloudformation">

Install Docker or Podman and Python 3, then follow the example's local deployment walkthrough.

```shell
atmos validate stacks
```

</TryExample>
````

Pass the complete project path relative to `examples/`, without a leading slash
or the `examples/` prefix. For nested projects, the working directory is the final
path segment. Do not pass individual files or directories that depend on a parent
project. The example must have a corresponding `/examples/<path>` page.

Optional children appear after the initialization commands in the example tab.
Include relevant prerequisites and a useful first command; keep the full walkthrough
in the example README.

On a command reference page, the quick start must run the command documented by
that page against a real target in the example. Do not reuse a generic inspection
command across a family of pages. Include prerequisite commands when needed (for
example, deploy before reading outputs), expected failures or empty results, and
cleanup for local resources. Verify commands in a fresh copy of the project.

For multiple examples in one section, wrap direct `TryExample` children in
`TryExamples` and give each a short `label`. This renders one **Install Atmos** tab
followed by the named example tabs, preserving each project's instructions:

```mdx
import TryExample, { TryExamples } from '@site/src/components/TryExample';

## Try it out

<TryExamples>
  <TryExample example="devcontainer" label="Dev Container">
    Install Docker or Podman, then run `atmos devcontainer list`.
  </TryExample>
  <TryExample example="devcontainer-build" label="Build Geodesic">
    Follow the README to build the Geodesic development container.
  </TryExample>
</TryExamples>
```

Individual `/examples/<path>` pages include a single `TryExample` above their
demo and README. They use `showExampleLink={false}` to omit links back to the
current page, and link to the README for prerequisites and follow-up commands.

Each instance has its own tabs, initially showing the example (the first named
example in a group). Installation remains available in the first tab. The command uses
`atmos init examples/<path>`, which defaults to the official Atmos repository on
`main`. User initialization settings can override the repository or revision;
the component links to that configuration reference.
