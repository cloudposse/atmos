---
title: Atmos FAQ
sidebar_label: FAQ
sidebar_position: 4
---

### Why is the tool called Atmos?

Once upon a time, in the vast expanse of cloud computing, there was a need for a tool that could orchestrate and manage the complex layers of
infrastructure with ease and precision. This tool would need to rise above the rest, providing oversight and control much like a protective layer
enveloping the Earth. Thus, the idea of "atmos" was born, drawing inspiration from the visionary science fiction of the "Aliens" movie, where
atmospheric terraformers transform alien worlds into hospitable realms.

But there's more to the story. As the [Cloud Posse](https://cloudposse.com) delved deeper into crafting this tool, they discovered a wonderful coincidence. "Atmos" could
stand for **"Automated Terraform Management & Orchestration Software,"** perfectly encapsulating its purpose and capabilities. This serendipitous
acronym was a delightful surprise, further cementing the name's destiny.

### Are there any reference architectures for Atmos?

Yes and no. Cloud Posse sells [reference architectures for AWS](https://cloudposse.com/services/) based on Atmos and Terraform. Funded Startups and Enterprises are the targets of these architectures.

We plan to release some free reference architectures soon but cannot commit to a specific date.

Until then, please review our [Quick Start](/quick-start/), which takes you through deploying your first Terraform components using Atmos.

### Is Atmos similar to Terragrunt?

Yes, Atmos is similar to Terragrunt in that it offers a robust command-line tool for operating with Terraform at scale. Both tools are designed to enhance Terraform's capabilities, allowing for more efficient management of complex infrastructure configurations. They support similar functionality, such as DRY (Don't Repeat Yourself) code practices, module management, and workflow orchestration, but they diverge significantly in their approach and philosophy.

Terragrunt was a pioneer in Terraform orchestration, adopting a highly opinionated approach. It leverages HCL (HashiCorp Configuration Language) to define an extension of Terraform that, behind the scenes, generates code and stitches modules together. This method streamlines and automates much of the boilerplate code required for large-scale Terraform deployments.

### How is Atmos unlike Terragrunt?

On the other hand, Atmos takes a distinct path. While it embraces the core principles of infrastructure as code and Terraform orchestration, Atmos opts for a more flexible and less prescriptive approach. It focuses on providing a more opinionated framework on top of a  user-friendly CLI experience that accommodates a wide range of workflows and infrastructure patterns, not just for Terraform. Atmos emphasizes features that cater to both straightforward and complex deployment scenarios without enforcing a rigid framework on users. It's especially adept for complex, regulated enterprise environments with it's support for Stack manifests. 

### Can Atmos be used together with Terragrunt?

Yes, technically, Atmos and Terragrunt can be used in conjunction. However, it's important to acknowledge that this combination introduces a significant overlap in functionality and some philosophical differences. Developers generally prefer to minimize the number of layers in their tooling to avoid complexity, and integrating these two tools could steepen the learning curve for teams. 

The key motivation for integrating Terragrunt within Atmos would be to offer a seamless CLI (Command Line Interface) experience, facilitating a gradual transition to Atmos's methodologies. This strategy allows teams to utilize Terragrunt for existing infrastructure management ("for historical reasons") while adopting Atmos for new projects. Essentially, it's a strategic approach to support migration to Atmos, enabling the tool to invoke Terragrunt where necessary, as teams progressively shift towards fully embracing Atmos for infrastructure orchestration. 

There are a few ways to accomplish it, depending on your needs or preferences.
1. Set the default the `command` to call in the Atmos CLI Configuration for Terraform components.
2. Override the `command` in the `settings` section for each component definition in the stack configuration (this respects inheritances, so the [`mixin`](/core-concepts/stacks/mixins) pattern can be used.)

### What are some of the alternatives to Atmos?

You can check out the [list of alternatives](/reference/alternatives) that have influenced its design.

### Is Atmos a replacement for Terraform?

No, atmos is an orchestration tool that supports terraform, along with other tools. Plus, it even supports
custom commands, so any tool can be powered by atmos.

### Does Atmos work with Terraform Cloud and Terraform Enterprise?

Probably, but we haven't tested it. 

Here's why it should work with any CI/CD system.
- Atmos works with vanilla terraform code
- Atmos simply generates `varfiles` from the deep-merged configurations for a given stack

### Does Atmos work with Atlantis?

Yes, it does. See our [integration page](/integrations/atlantis). 

### Does atmos support OpenTofu (OpenTF)

Yes, it does.

### Does Atmos only work with AWS?

No, **Atmos is entirely cloud-agnostic**. There are funded startups and enterprises alike, using Atmos on Amazon Web Services (AWS),
Google Compute Cloud (GCP) and Microsoft Azure (Azure), among others.

### Is Atmos a commercial product?

Atmos is free and open source under the permissive APACHE2 license.

It is primarily supported by [Cloud Posse](https://cloudposse.com/), a DevOps Accelerator for Funded Startups and Enterprises.

### How do you money?

[Cloud Posse](https://cloudposse.com/) sells [reference architectures for AWS](https://cloudposse.com/services/) that are based on Atmos and Terraform. 
Our typical customers are <strong>Funded Startups</strong> and <strong>Enterprises</strong> that want to leverage AWS together with platforms 
like GitHub, GitHub Actions, Datadog, OpsGenie, Okta, etc.
