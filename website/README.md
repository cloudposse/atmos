# Atmos Documentation

This folder contains the website powering official [Atmos](https://atmos.tools) documentation.

## Getting Started

1. Install dependencies by running `brew bundle install`. This command will look for the `Brewfile` and install its
   contents.
2. Install Node.js dependencies by running `pnpm install`.
3. Generate the atmos-manifest JSON Schema by running `atmos dev generate schema` from the repo root. This file is
   gitignored and generated from the embedded schema (`pkg/datafetcher/schema/atmos/manifest/1.0.json`), the single
   source of truth — it won't exist in a fresh checkout until you run this.
4. Build the local search base by running `pnpm run build`.
5. Start the local web server by running `pnpm start`.

The shortcut for running all these commands is just to run `make all`.

### Why pnpm?

We use [pnpm](https://pnpm.io/) for better performance and more efficient disk space usage. pnpm creates a single, shared store for packages and uses hard links to reference them in your project, making installs faster and saving disk space.
