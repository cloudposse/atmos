---
id: introduction
slug: /
sidebar_position: 1
title: Introduction
---

# Scale Terraform Workflows with Atmos

Businesses succeed because of their processes. Those processes need to be captured through workflows.

The reason is that workflows give teams superpowers without needing to be superheroes. Let's face it: human error is the number one risk businesses face. It's the root cause of most outages and can strike anytime.

The best workflows enable anyone, anywhere, to run them. They are easily understood, self-documenting and ensure fewer mistakes get made. When workflows are written down, they can be iterated upon and optimized. 

# Atmos is a workflow automation tool

With so many of these tools today, why does the world need another "workflow" automation tool?

It helps to understand some background. Atmos is a tool created by [Cloud Posse](https://cloudposse.com). We are [DevOps Accelerator for Startups](https://cloudposse.com/services/) (typically venture-backed, later-stage series A-D companies) and enterprises. We help our customers own their infrastructure using our [reference architecture blueprints](https://cloudposse.com/reference-architecture/) and implement it with their teams while showing them the ropes. We needed to scratch our own itch. After trying everything (`make`, `variant`, `mage`, `gotask`, `terragrunt`, etc), we realized it would not be possible to achieve the outcome we wanted for our customers using a mashup of existing off-the-shelf tools. 

## We Built Atmos to Solve Our Problems

Cloud Posse works with many customers who have a diverse set of requirements. We couldn't consistently deliver the outcomes we wanted using the existing tools on the market. Since everything we build at Cloud Posse is designed to be reusable across organizations, we needed something that allowed us to replicate configurations without repetitive steps and editing.

Here are some of the problems we needed to solve...
- how to **support many teams** concurrently across multiple organizations.
- how to **automate our deliverables** so that teams could *own* them.
- how to engineer an **extensible tool** that could handle unknown-unknown requirements.
- how to design an **easy to use tool** that was stable and consistent (aka testable)

## Atmos CLI: Handle Configuration @ Scale

Most tools suffer when configurations get large and complex. They are not optimized for this; Terraform is a prime example.

Atmos solves this by introducing powerful capabilities most tools lack.

- Hierarchical configurations to reduce complexity
- DRYness with inheritance and imports
- Team collaboration with reusable service catalogs
- Reusable workflows that run anywhere
- Consistent command line interface that can combine multiple tools into one command (`atmos`)
- A [single binary, cross-compiled for every architecture](https://github.com/cloudposse/atmos/releases) (golang) and [easily installed](/docs/quick-start/install).

## YAML Configuration

So, why did we choose YAML? That's simple: **everyone knows YAML.**

YAML is an extensible markup language. It's ideal for defining a declarative DSL.

Using YAML, `atmos` is easily able to support features like imports, inheritance, deep merging, and policies.

YAML is also "tool agnostic" (unlike HCL); it's not tied to Terraform or explicitly associated with HashiCorp products.
- **Every language can read it** (e.g. from python or node. Even read it remotely, e.g. via http)
- **Every language can write it** (e.g. generate it from a web UI)
  
There are other cool things we get "for free" by adopting YAML. 
- [YAML anchors and aliases](https://yaml.org/spec/1.2.2/#3222-anchors-and-aliases) can help DRY up configurations
- Easy [OPA validation](/docs/core-concepts/components/component-validation#open-policy-agent-opa)
- [JSON Schema validation](/docs/core-concepts/components/component-validation#json-schema)

The possibilities are endless. 

## We Need Guardrails

Configuration "at scale" requires policies and validation. Making mistakes is only to be human. We need to clarify what's expected and "shift left" so that errors are raised early.

Atmos supports OPA to lay down the law and enforce it.

For example, here are some things that are possible:
- Never allow some value in a particular environment
- Never allow two types of things to be deployed concurrent in staging
- Never allow a team to deploy some component
- Never allow production to use unstable versions of components

The possibilities are endless. 

## Inheritance is Powerful

Make configuration hierarchical with imports and deep merging.
Organize it anyway that make sense for your organization. 
Define logical groups of configuration
Define something once and reuse it. 
Know exactly where configuration differs. 


## Generalized Tools Always Suffer

However, a general tool will always suffer for a specialized use case, even atmos and it’s custom workflows
So that’s why we wrote it in Golang
And implemented opinionated workflows for the tools we use like terraform and helmfile 
We also acknowledged that not everyone knows go, so we created a porcelain style plugin interface 
Write your commands in any language. Run them via atmos using a standard CLI UX
Or better yet, combine them
We like to think this tool gives us superpowers. It’s definitely attributable to our meteoric success of the past couple years and we want that to be yours too. 

## Terraform by itself does not scale

There are many problems encountered by teams working with Terraform at scale. 

- Terraform GitOps (CI/CD) for teams is non-trivial
- Terraform configuration is not DRY
- Terraliths are a terraform anti-pattern, yet…
- Terraform “root modules” are not easily shared
- Terraform has no native support for multiple root modules (dependency ordering)
- Terraform backend configurations cannot be parameterized
- Terraform lacks built-in guardrails & policy controls that enterprises need


## Terraform + Atmos is the answer
Opinionated workflows for terraform
Established conventions and patterns to manage terraform @ scale
Break terraliths into smaller pieces (“components”)
Enable team collaboration with catalogs and libraries
Enforce guardrails with OPA and JSON Schema
Zero Vendor Lock-in

## Terminology

Let's get some terminology in place so we can talk about some specifics of `atmos`.

**Components**: these are the fundamental building blocks. For terraform, these are opinionated terraform “root modules”.
**Stacks**: these are the combination of components into an architecture.
**Catalogs**: these are collections of reusable stack configurations.

## Atmos Gives Terraform Super Powers

Write components (root modules) once, reuse any number of times
Vanilla HCL without customizations
Use our terraform provider to read YAML configurations from anywhere 
Easily deploy across any number of regions without touching a line of HCL
Duplicate entire stacks by just copying a single YAML file
Generate YAML configurations e.g. Cookie Cutter, Service Now


## TL;DR

- Businesses need to capture their processes as workflows
- Terraform configuration is difficult to manage at scale
- Terraform by itself is not enough to be successful
- Atmos makes managing those configurations easier

## It’s Open Source (APACHE2)

Best of all Atmos is open-source with an active community supporting it. There are no strings attached and it's under active development with hundreds of stars. 

There's also an ecosystem of tools that it works with, and Terraform modules downloaded tens of millions of times. 

We invite you to join us in the larger movement and impact the the industry at large. We offer a greater, more impactful.

## Next Steps

* Do you need to present a compelling case for Atmos at your company? [Check out our slides](/reference/slides).
* [Check out our Quick Start](/category/quick-start)
* [Review the Core Concepts](/category/core-concepts)
