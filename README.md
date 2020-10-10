# variants

Universal CLI for DevOps and Cloud Automation.


## Introduction

`variants` is a command-line tool for provisioning, managing and orchestrating cloud infrastructure and [Kubernetes](https://kubernetes.io/) clusters.

It includes commands for:

  - Provisioning [Terraform](https://www.terraform.io/) projects
  - Deploying [helm](https://helm.sh/) [charts](https://helm.sh/docs/topics/charts/) to Kubernetes clusters using [helmfiles](https://github.com/roboll/helmfile)
  - Executing [helm](https://helm.sh/) commands on Kubernetes clusters
  - Provisioning [istio](https://istio.io/) on Kubernetes clusters using [istio operator](https://istio.io/latest/blog/2019/introducing-istio-operator/) and helmfile
  - Executing shell commands
  - Combining commands into workflows to execute many commands sequentially in just one step
  - ... and many more

The CLI is built with [variant2](https://github.com/mumoshu/variant2) using [HCL syntax](https://www.terraform.io/docs/configuration/index.html).

`*.variant` files are combined like Terraform files.  Separating the files into [modules](modules) is done for cleanliness. 

See `variant` docs for more information on [writing commands](https://github.com/mumoshu/variant2#writing-commands).


## Usage

Run `opsctl help` to see the available commands and available flags.

### Provision Terraform Project

To provision a Terraform project using the `opsctl` CLI, run the following commands in the container shell:

```bash
opsctl terraform plan eks --environment=ue2 --stage=dev
opsctl terraform apply eks --environment=ue2 --stage=dev
```

where:

  - `efs` is the Terraform project to provision (from the `projects` folder)
  - `--environment=ue2` is the environment to provision the project into (e.g. `ue2`, `uw2`). Note: the environments we are using here are abbreviations of AWS regions
  - `--stage=dev` is the stage/account (`prod`, `staging`, `dev`)

Short versions of the command-line arguments can be used:

```bash
opsctl terraform plan eks -e ue2 -s dev
opsctl terraform apply eks -e ue2 -s dev
```

To execute `plan` and `apply` in one step, use `terrafrom deploy` command:

```bash
opsctl terraform deploy eks -e ue2 -s dev
```

### Provision Helmfile Project

To provision a helmfile project using the `opsctl` CLI, run the following commands in the container shell:

```bash
opsctl helmfile diff ingress-nginx --environment=ue2 --stage=dev
opsctl helmfile apply ingress-nginx --environment=ue2 --stage=dev
```

where:

  - `ingress-nginx` is the helmfile project to provision (from the `projects/helmfiles` folder)
  - `--environment=ue2` is the environment to provision the project into (e.g. `ue2`, `uw2`). Note: the environments we are using here are abbreviations of AWS regions
  - `--stage=dev` is the stage/account (`prod`, `staging`, `dev`)

Short versions of the command-line arguments can be used:

```bash
opsctl helmfile diff ingress-nginx -e ue2 -s dev
opsctl helmfile apply ingress-nginx -e ue2 -s dev
```

To execute `diff` and `apply` in one step, use `helmfile deploy` command:

```bash
opsctl helmfile deploy ingress-nginx -e ue2 -s dev
```

### Docker Access

The geodesic `infrastructure` image builds the CLI through the normal build process. Build and run the image as normal to access the `opsctl` cli.


