---
title: Getting Started with Atmos
sidebar_label: Getting Started with Atmos
sidebar_position: 1
description: "Learn what Atmos is and how you can start using it with stacks to simplify your DevOps Automation tasks."
---

## Introduction

Atmos is part of the SweetOps toolchain and was built to make DevOps and Cloud automation easier across multiple tools. It has direct support for
automating Terraform, Helm, Helmfile, and Istio. By natively utilizing [stacks](/core-concepts/stacks), `atmos` enables you to effortlessly manage
your Terraform and Helmfile [components](/core-concepts/components) from your local machine or in your CI/CD pipelines.

In this tutorial we'll be looking at a simple (albeit contrived) example of automating multiple Terraform components together into a workflow. This
will give you some understanding of what `atmos` can do while also giving you some experience with using it at the command line.

## Prerequisites

### System Requirements

To accomplish this tutorial, you'll need to have [Git](https://git-scm.com/book/en/v2/Getting-Started-Installing-Git)
and [Docker](https://docs.docker.com/get-docker/) installed on your local machine.

**That's it**.

### Understanding

Prior to starting this tutorial, you should be sure you understand [our various concepts and terminology](/core-concepts)) and have gone
through our [Getting started with Geodesic](https://docs.cloudposse.com/tutorials/geodesic-getting-started/) tutorial because we'll be using Geodesic
as our means to run `atmos`.

## Tutorial

### 1. Clone the tutorials repository, then run the `tutorials` image

As part of this tutorial (and others following in our tutorial series), we will utilize [our tutorials](https://github.com/cloudposse/tutorials)
repository](https://github.com/cloudposse/tutorials). The repository includes code and relevant materials for you to use alongside this tutorial
walkthrough.

Let's clone it to your local machine and `cd` into it:

```bash
git clone git@github.com:cloudposse/tutorials.git

cd tutorials
```

Now that we've got our code, we'll want to interact with it using our standard set of tools. Following the SweetOps methodology, we will use a docker
toolbox, a Docker image built on top of Geodesic. This entire `tutorials` repository is actually Dockerized to make that part easy, so let's run
our `cloudposse/tutorials` image:

```bash
# Run our docker image
docker run -it \
          --rm \
          --volume "$HOME":/localhost \
          --volume "$PWD":/tutorials \
          --name sweetops-tutorials \
          cloudposse/tutorials:latest;
```

This command will pull the `tutorials` image to your local machine, run a new container from that image, and mount the various tutorial folders so you
can edit them on your host machine or in the container and the changes will propogate either direction.

![Tutorial Shell](/img/tutorials-3-tutorials-shell.png)

Now that we're running inside our container, let's get into our specific tutorial directory:

```bash
cd /tutorials/02-atmos
```

This `02-atmos/` directory should look like the following:

```
.
├── README.md
├── components/
└── stacks/
```

### 2. Confirm our tools are working

Now that we have an interactive bash login shell open into our `cloudposse/tutorials` image with our home folder, `stacks/`, and `components/`
directories all mounted into it, let's check that all is working correctly by invoking a couple commands to make sure things are install correctly:

```bash
terraform -v # Should return: Terraform vX.X.X

atmos version # Should return a simple Semver number.
```

Awesome! We've now successfully seen our first `atmos` command, and we're ready to start using it!

### 3. Terraform plan and apply a component

Now that we've got access to `atmos`, let's do something simple like execute `plan` and `apply` on some terraform code! To do that, we need two
things:

1. Components -- We've provided 3 small example components in our `components/terraform/` directory, which is mounted to `/tutorials/02-atmos/` inside
   your running container.
1. A Stack configuration -- We've provided a simple example stack located at `stacks/example.yaml`. This is similarly mounted
   to `/tutorials/02-atmos/` inside your running container.

For our example in this step, we'll check out the `components/terraform/fetch-location` component. To plan that component, let's execute the
following:

```bash
atmos terraform plan fetch-location --stack=example
```

If you correctly entered your command, you should see a successful plan which resulted in "Terraform will perform the following actions" followed by "
Changes to Outputs." You'll notice this first executes a `terraform init` before doing the plan. This is intentional to ensure `atmos` can be invoked
without prior project setup. Note, we'll discuss the currently unknown `--stack` parameter shortly.

So now that we've done a plan... let's get this project applied. We could invoke `atmos terraform apply ...`, but our best option at this point would
be to invoke `deploy` which will execute a terraform `init`, `plan`, and `apply` in sequential order:

```bash
atmos terraform deploy fetch-location --stack=example
```

Even though this component didn't have any resources, your deploy’s `apply` step will utilize
the [`http`](https://registry.terraform.io/providers/hashicorp/http/latest/docs/data-sources/http) data source to invoke a request
to `https://ipwhois.app/json/` and output your city, region, country, and latitude + longitude (found by your IP address).

Awesome, we've got a component applied, but that would've been pretty trivial to do without `atmos`, right? We consolidated down 3 commands into one
which is great, but we can do a lot better... Let's show you where `atmos` really provides value: Workflows.

### 5. Invoke an Atmos Workflow

The SweetOps methodology is built on small, composable components because through experience practitioners have found large root modules to become
cumbersome: They require long `plan` times, create large blast radiuses, and don't foster reuse. However, the tradeoff with smaller root
modules ([components](/core-concepts/components)) is that you then need to orchestrate them in an order that makes sense for what you're building.
That is where `atmos` workflows come in. Workflows enable you to describe the ordering of how you want to orchestrate your terraform or helmfile
components so that you can quickly invoke multiple components via one command. Let's look at an example in our `/stacks/example.yaml` file:

```yaml
vars: { }

terraform:
  vars: { }

helmfile:
  vars: { }

components:
  terraform:
    fetch-location:
      vars: { }

    fetch-weather:
      vars: { }

    output-results:
      vars:
        print_users_weather_enabled: true

  helmfile: { }

workflows:
  deploy-all:
    description: Deploy terraform projects in order
    steps:
      - command: terraform deploy fetch-location
      - command: terraform deploy fetch-weather
      - command: terraform deploy output-results
```

Here we can see our first stack, so let's break this file down to help understand what it is doing:

1. We've got a couple of empty elements at the top: `import` and `vars`. We'll address these in an upcoming tutorial.
1. We've got `terraform` and `helmfile` elements that have empty `vars` elements. These provide any shared configuration variables across components
   when dealing with more complicated stacks. We'll address these in an upcoming tutorial as well.
1. We've got our `components` element which has `terraform` and `helmfile` elements. This is where we describe our various components that make up our
   stack and the input configurations that we want to invoke them with (via their `vars` elements). You can see here we have our 3 terraform
   components from within our `components/terraform/` folder specified here and some configuration to go along with them.
1. Finally, we've got our `workflows` element. This is a `map` type element that accepts a workflow name as the key and then the description and steps
   as values. In the example `deploy-all` workflow, our steps are `job` items which describe to `atmos` that we want to run `atmos terraform deploy`
   on each component in our stack.

To sum it up, our stack represents an environment: It describes the components we need to deploy for that environment, the configuration we want to
supply to those components, and finally the ordering of how to orchestrate those components. This is immensely powerful as it enables us to provide
one source of truth for what goes into building an environment and how to make it happen.

Now that we know what is in our `example.yaml` stack configuration, let's invoke that workflow:

```bash
atmos workflow deploy-all -f example.yaml -s example
```

This will run our various steps through `atmos` and you should see the sequential `init`, `plan`, and `apply` of each component in the workflow to
output the current weather for your area. We hope it's sunny wherever you're at 😁 🌤

Let's move on to updating our code and getting a feel for working a bit more hands on with `atmos` and stacks.

### 6. Update our Stack

One of the critical philosophies that SweetOps embodies is a focus
on [improving Day 2+ operations](https://docs.cloudposse.com/fundamentals/philosophy/#optimize-for-day-2-operations) and with that in mind, it's
important to know how you would update this stack and utilize `atmos` to make those changes. Luckily, that's as simple as you might think. Let's try
it out and update the `stacks/example.yaml` file on our local machines to the following:

```yaml
vars: {}

terraform:
  vars: {}

helmfile:
  vars: {}

components:
  terraform:
    fetch-location:
      vars: {}

    fetch-weather:
      vars:
        # Let's get the weather for a particular day.
        # Feel free to update to a date more relevant to you!
        date: 2021/03/28

    output-results:
      vars:
        print_users_weather_enabled: false # We disable outputting via our Terraform local-exec.

  helmfile: {}

workflows:
  deploy-all:
    description: Deploy terraform projects in order
    steps:
      - command: terraform deploy fetch-location
      - command: terraform deploy fetch-weather
      - command: terraform deploy output-results
```

Above, we updated a couple of variables to change the behavior of our terraform code for this particular stack. Since we mounted our local `stacks/`
folder to our Atmos container via `--volume` argument, when you save the above stack file, Docker will update your container's 
`/stacks/example.yaml` file as well.

Now to execute this again... we simply invoke our `deploy-all` workflow command.
This should run through our workflow similar to the way we did it before. Still, this time we'll see our temperature return from the weather API for
the date you specified instead of today's date. We'll skip over our terraform `local-exec`'s `echo` command for "pretty printing" our weather data.
Instead, we'll just get our updated weather information as one of our `Outputs`.

## Conclusion

Wrapping up, we've seen some critical aspects of SweetOps in action as part of this tutorial:

1. Another usage of Geodesic to easily provide a consistent environment where we have easy access to tools (like `atmos` and `terraform`).
1. An example stack along with the breakdown of what goes into a stack and why it is a powerful way to describe an environment.
1. Example components that require a specific workflow in order to execute correctly.
1. Usage of Atmos in executing against some terraform code and orchestrating a workflow from our stack.

With these tools, you can skip documenting the various steps of building an environment (aka WikiOps) and instead focus on just describing and
automating those steps! And there are many more Atmos features that can do beyond this brief intro, so keep looking around the docs for more
usage patterns!

Want to keep learning but with a more real-world
use-case? [Check out our next tutorial on deploying your first AWS environment with SweetOps](/tutorials/first-aws-environment).
