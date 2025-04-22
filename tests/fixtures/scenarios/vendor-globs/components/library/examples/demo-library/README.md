# Demo Components

Here are some examples of distributing reusable components as part of a library.

Typically, a component library will be a separate repository containing only components with a monorepo design.

## Examples

These examples are somewhat contrived and selected mainly because they use remote APIs that do not require authentication.

> ![TIP]
> These examples are more representative of proper child modules rather than "root modules". 
> Remember, root modules are stateful pieces of your architecture, meaning they are Terraform root modules with a state backend.
> Typical root modules include networks, clusters, databases, caches, object stores, load balancers, and so on.
> For a real-world example of the components we use in Cloud Posse’s AWS Reference Architecture, please see [`cloudposse/terraform-aws-components](https://github.com/cloudposse/terraform-aws-components).


### GitHub

The [`github/*`](github/) example components use the `http_provider` to anonymously query the GitHub API for information.

### Weather

The [`weather`](weather/) example component requests weather data from `wttr.in` based on the location provided.

### IP Info

The [`ipinfo`](ipinfo/) example component returns information about your current IP.


