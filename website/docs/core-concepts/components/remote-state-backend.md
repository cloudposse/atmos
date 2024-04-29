---
title: Remote State Backend
sidebar_position: 16
sidebar_label: Remote State Backend
id: remote-state-backend
---

Atmos supports configuring [Terraform Backends](/core-concepts/components/terraform-backends) to define where
Terraform stores its [state](https://developer.hashicorp.com/terraform/language/state) data files,
and [Remote State](/core-concepts/components/remote-state) to get the outputs
of a [Terraform component](/core-concepts/components), provisioned in the same or a
different [Atmos stack](/core-concepts/stacks), and use
the outputs as inputs to another Atmos component

Atmos also supports Remote State Backends (in the `remote_state_backend` section), which can be used to configure the
following:

- Override [Terraform Backend](/core-concepts/components/terraform-backends) configuration when accessing the
  remote state of a component (e.g. override the IAM role to assume, which in this case can be a read-only role)

- Configure a remote state of type `static` which can be used to provide configurations for Atmos components for
  [Brownfield development](https://en.wikipedia.org/wiki/Brownfield_(software_development))

## Override Terraform Backend Configuration

## `static` Remote State for Brownfield development

[Brownfield development](https://en.wikipedia.org/wiki/Brownfield_(software_development)) is a term commonly used in the
information technology industry to describe problem spaces needing the development and deployment of new software
systems in the immediate presence of existing (legacy) software applications/systems. This implies that any new software
architecture must take into account and coexist with the existing software. 

Similarly, in Atmos, brownfield development describes the process of configuring Atmos components and stacks for the
existing (already provisioned) resources.

