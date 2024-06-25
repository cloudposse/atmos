# Examples

> [!TIP]
  > ### You can try `atmos` directly in your browser using GitHub Codespaces!
  >
  > [![Open in GitHub Codespaces](https://github.com/codespaces/badge.svg)](https://github.com/codespaces/new?hide_repo_select=true&ref=main&repo=cloudposse/atmos&skip_quickstart=true)
  >
  > <i>Already start one? Find it [here](https://github.com/codespaces).</i>
  >

## Try it Locally

To play with these demos locally, start by [installing `atmos`](https://atmos.tools/install). 

Then run the following commands (Note, these commands requires that you have `curl` and `tar` installed on your system.)

```shell
atmos demo download <example>
```

For example, the following command will download the `demo-stacks` example.
```shell
atmos demo download demo-stacks
```

> [!TIP]
> #### Fun Fact
>
> The `download` command is a [custom command](https://atmos.tools/core-concepts/custom-commands) added to the default `atmos.yaml`.
>


## Demos

We designed the demos to be basic examples showcasing functionality. Expect some redundancies and overlap since we reuse examples to demonstrate specific behaviors. Each demo focuses on one area to reduce complexity and make it easier to grasp the concepts.

Think of each demo folder as representing an example of a standalone repository. To make it easier, we put all the demos in one place.

```shell
1. ├── demo-stacks/            # Start your journey here
2. ├── demo-library/           #
3. ├── demo-validation/        #
4. ├── demo-vendoring/         #
5. ├── demo-custom-commands/   #
6. └── demo-workflows/         #
```

## Playground


## Quick Start (Simple)

The [`demo-stacks`](demo-stacks/) contains code for the [simple quick start](https://atmos.tools/quick-start/advanced). This requires no special permissions and is the fastest way to grasp [atmos concepts](https://atmos.tools/core-concepts).

## Quick Start (Advanced)

The [`quick-start`](quick-start/) contains code for the [advanced quick start](https://atmos.tools/quick-start/advanced). Please note that it requires an AWS organization with administrative credentials.

## Tests

The [`tests/`](tests/) folder includes a variety of patterns and configurations that we use for CI tests. They are not intended to serve as examples or best practices. We deliberately break some of them for testing purposes. These configurations implement every function of Atmos in some shape or form, but they are not necessarily exemplars of design.
