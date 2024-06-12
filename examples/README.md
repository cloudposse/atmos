# Examples

## Demos

We designed the demos to be basic examples showcasing functionality. Expect some redundancies and overlap since we reuse examples to demonstrate specific behaviors. Each demo focuses on one area to reduce complexity and make it easier to grasp the concepts.

Think of each demo folder as representing an example of a standalone repository. To make it easier, we put all the demos in one place.

```shell
├── demo-library/
├── demo-stacks/
│   ├── components/
│   │   └── terraform/
│   │       └── myapp/
│   └── stacks/
│       ├── catalog/
│       └── deploy/
├── demo-validation/
├── demo-vendoring/
├── demo-custom-commands/
└── demo-workflows/
    ├── stacks/
    │   ├── catalog/
    │   └── deploy/
    └── workflows/
```

## Playground

To play with these demos, start by [installing atmos](https://atmos.tools/install). Note, this requires you have `curl` and `tar` installed on your system.

Then run, 

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

## Quick Start (Simple)

The [`demo-stacks`](demo-stacks/) contains code for the [simple quick start](https://atmos.tools/quick-start/advanced). This requires no special permissions and is the fastest way to grasp [atmos concepts](https://atmos.tools/core-concepts).

## Quick Start (Advanced)

The [`quick-start`](quick-start/) contains code for the [advanced quick start](https://atmos.tools/quick-start/advanced). Please note that it requires an AWS organization with administrative credentials.

## Tests

The [`tests/`](tests/) folder includes a variety of patterns and configurations that we use for CI tests. They are not intended to serve as examples or best practices. We deliberately break some of them for testing purposes. These configurations implement every function of Atmos in some shape or form, but they are not necessarily exemplars of design.
