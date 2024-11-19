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

Then, clone this repo and [try out the demos](https://github.com/cloudposse/atmos/tree/main/examples).

```shell
# Clone this repo
git clone git@github.com:cloudposse/atmos.git

# Try the examples: https://github.com/cloudposse/atmos/tree/main/examples
cd examples/
```

## Demos

We designed the demos to be basic examples showcasing functionality. Expect some redundancies and overlap since we reuse examples to demonstrate specific behaviors. Each demo focuses on one area to reduce complexity and make it easier to grasp the concepts.

Think of each demo folder as representing an example of a standalone repository. To make it easier, we put all the demos in one place.

```shell
1.  ├── demo-stacks/              # Kickstart your journey by exploring stack configurations and their structure.
2.  ├── demo-library/             # Explore a reusable component library designed for seamless vendoring.
3.  ├── demo-vendoring/           # Learn how to use vendoring to download and integrate remote dependencies from the `demo-library`.
4.  ├── demo-validation/          # Validate your configurations to ensure correctness and compliance.
5.  ├── demo-localstack/          # Leverage LocalStack to provision an S3 bucket using Atmos and Terraform.
6.  ├── demo-helmfile/            # Deploy NGINX on a local lightweight Kubernetes cluster (k3s) using Helm.
7.  ├── demo-custom-command/      # Learn how to extend Atmos with your own custom CLI commands.
8.  ├── demo-component-versions/  # Discover how to manage and use multiple versions of components effectively.
9.  ├── demo-context/             # Simplify resource naming and tagging with our Terraform context provider.
10. └── demo-workflows/           # Automate repetitive tasks with streamlined workflows.
```

## Playground


## Quick Start (Simple)

The [`demo-stacks`](demo-stacks/) contains code for the [simple quick start](https://atmos.tools/quick-start/advanced). This requires no special permissions and is the fastest way to grasp [atmos concepts](https://atmos.tools/core-concepts).

## Quick Start (Advanced)

The [`quick-start`](quick-start/) contains code for the [advanced quick start](https://atmos.tools/quick-start/advanced). Please note that it requires an AWS organization with administrative credentials.

## Tests

The [`tests/`](tests/) folder includes a variety of patterns and configurations that we use for CI tests. They are not intended to serve as examples or best practices. We deliberately break some of them for testing purposes. These configurations implement every function of Atmos in some shape or form, but they are not necessarily exemplars of design.
