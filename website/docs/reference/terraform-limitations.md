---
title: Terraform Limitations
description: Terraform Limitations
sidebar_label: Terraform Limitations
sidebar_position: 6
---

# Terraform Limitations

To grasp the motivation behind Atmos's creation, it's helpful to examine Terraform—a tool integral to our development processes. 
Let's delve into the challenges and limitations we encountered with Terraform, setting the stage for Atmos's development.

## What is Terraform?

Terraform is a command-line utility that processes infrastructure configurations in ["HashiCorp's Configuration Langauge" ("HCL")](https://en.wikipedia.org/wiki/HCL) to orchestrate infrastructure provisioning. Its chief role is to delineate and structure infrastructure definitions.
  
Terraform's HCL started strictly as a configuration language, not a markup or programming language, although has evolved considerably over the years. HCL is backward compatible with JSON, although it's not a strict superset of JSON. HCL is more human-friendly and readable, while JSON is often used for machine-generated configurations. This means you can write Terraform configurations in HCL or JSON, and Terraform will understand them. This feature is particularly useful for generating configurations programmatically or interoperation with systems that already use JSON.

## How has Terraform HCL Evolved?

As Terraform progressed and HCL evolved, notably from version _0.12_ onwards, HCL began incorporating features typical of programming languages (albeit without a debugger!). This shift enriched infrastructure definitions, positioning HCL more as a domain-specific programming language for defining infrastructure than strictly a configuration language (aka data interchange formats like JSON). As a result, the complexity of configuring Terraform projects has risen, while Terraform's inherent capabilities to be configured haven't evolved at the same pace.

- **Rich Expressions:** Introduced a richer expression syntax, removing the need for interpolations.

- **For Loops and Conditionals:** Added for expressions and conditional expressions.

- **Type System:** Introduced a more explicit type system for input and output values.

## Why is additional tooling needed when using Terraform?

**Every foundational tool begins simply.**

As users grow more advanced and their ambitions expand, the need for advanced tooling emerges. These shifts demonstrate that core technologies naturally progress, spawning more advanced constructs to tackle increased intricacies and enhance efficiency -- all while retaining their core essence. Just as CSS, JavaScript, Docker, Helm, and many other tools have evolved to include higher-order utilities, Terraform, too, benefits from additional orchestration tools, given the complexities and challenges users face at different stages of adoption.

Examples of tools like these are numerous, like

- **CSS has Sass:** Sass provides more expressive styling capabilities, variables, and functions, making stylesheets more maintainable and organized, especially for large projects.
- **JavaScript has TypeScript:** TypeScript brings static typing to JavaScript, improving code robustness, aiding in catching errors at compile-time, and better supporting large-scale applications.
- **Docker has Docker Compose:** Docker Compose simplifies the management and orchestration of multi-container Docker applications, making it easier to define, run, and scale services collectively.
- **Helm charts have Helmfiles.** While Helm charts define the blueprints of Kubernetes services, Helmfiles enable better orchestration, management, and deployment of multiple charts, similar to coordinating various instruments in a symphony.
- **Kubernetes manifests have Kustomize:** Kustomize allows customization of Kubernetes manifests without changing their original form, facilitating dynamic configurations tailored to specific deployment scenarios.

When considering Terraform in the context of large-scale organizations or enterprises, it's clear that Terraform and its inherent language don't address all challenges. With thousands of components spread across hundreds of accounts, cloud providers and managed by a vast number of DevOps engineers and developers, the complexity becomes overwhelming and difficult to manage.

A lot of the same challenges faced by CSS, Javascript, Docker, Helm and Kubernetes also exist in Terraform as well.

- Making modules more maintainable and organized, especially for large projects
- Better support for large-scale service-oriented architectures
- Easier ways to define, run, and scale services collectively
- Better orchestration, management, and deployment of multiple services

Here's a more exhaustive list:

- **Lack of DRY Configurations**: Terraform does not inherently support hierarchical configurations. There's no support for [deep merging configurations](https://github.com/hashicorp/terraform/issues/24987), making manual `varfile` maintenance unscalable. This makes it more difficult to enforce organizational standards, security controls, tagging, and policies.
- **State Management**: Managing Terraform's state, especially at scale, lacks inherent strategies for handling complexities such as access controls, multi-region, and Disaster Recovery (DR).
- **Limited Modularization**: Structuring configurations modularly while promoting code reuse is cumbersome.
- **Manual Initialization**: Backend initialization, module downloads, and other setup tasks require manual steps before executing `terraform apply`. This ties into the need for some kind of workflow tool.
- **Dependency Management**: Community edition of Terraform doesn't provide any mechanisms for orchestrating dependencies among root modules.
- **Absence of Stack Management**: Organizing configurations into structured stacks isn't a built-in feature of the community edition.
- **Lack of Automatic Dependency Ordering**: Standalone Terraform doesn't inherently determine execution order based on inter-stack dependencies.
- **No Native Workflow Automation and Standardization**: Dynamic workflow executions, such as having a unified workflow both in CI/CD platforms like GitHub Actions (GHA) and locally, are not inherently supported. Workflow standardization and automation capabilities do not exist, making provisioning and management tasks more manual, or relying on custom scripts, Makefiles, or other tooling.
- **Basic Environment Management**: Managing configurations across multiple environments can become complex without higher-level tooling.

For each of these challenges, a tailored solution is essential. Ultimately, the goal is to make Terraform more scalable, maintainable, and developer-friendly, especially in complex and large-scale environments.

HashiCorp primarily offers foundational guidance on Terraform and pushes companies instead toward Terraform Enterprise. In fact, it's held back features from entering into the Terraform core that would make it more standalone. HashiCorp does not thoroughly address how to solve the above challenges using Terraform. While suitable for some, it may not meet the scalability demands of enterprise, especially as they embark on their Terraform adoption journey.

## What is the natural progression of Terraform adoption?

Every advancement in tool adoption is accompanied by new challenges, introducing complexities that are justified by the evolution's benefits. Indeed, the true value of these evolutions often only becomes clear once we're faced with the intricate challenges they address. Here's what a typical path of adoption looks like for organizations adopting Terraform.

### **Stage 1:** Introduction to Terraform, *Hello World!*

1. Developers roll up their sleeves and get a simple terraform example up and running. No modules are used and mostly hardcoded resources with settings.
2. Local state files (since this is just an exploration)
3. No Version Control System (VCS) is used, just stored locally on workstation.

:::warning New Problems
1. How do we handle secrets?
2. Where do we store the state file?
3. How do we maintain something with so many hardcoded settings?
:::

### **Stage 2:** The Monolithic Root Module (aka Terralith)

1. Developers begin by composing a single root module that continuously expands.
2. Define all environments as code (dev, staging, production) in a single Terraform root module, and use feature flags for everything (e.g. `prod_cluster_size`, `staging_cluster_size`, etc)

:::warning New Problems
1. A million parameters for each environment to toggle settings.
2. Impossible to test every combination of parameters.
3. Not very DRY. Lots of code duplication.
:::

### **Stage 3:** Move Towards Modularization

1. Developers realize that a lot of code is getting copied and pasted, so they advance to writing modules to avoid duplication.
2. Modules act like functions in Terraform: reusable and parameterized. They can be called multiple times and accept parameters.
3. It works well for now, and developers succeed in bringing up all the environments. It certainly beats ClickOps.

:::warning New Problems
1. These modules are "reusable" but usually just within the project itself and not written to be reused across the organization.
2. Many assumptions are made on specific requirements of the problem at hand, and generalizing them is not of paramount concern.
3. Lots of similar modules appear in the organization, some more maintained than others.
4. Still, no automated tests (E.g. `terratest`).
5. Everyone is probably an Administrator.
:::

### **Stage 4:** Adoption of Open Source Modules

1. Developers learning terraform, frequently start by writing their own modules. Frequently, they are even unaware that a huge module ecosystem already exists (like [Cloud Posse's terraform modules](https://github.com/cloudposse))
2. Now, aware of well-maintained open-source modules, they start ripping out their custom modules (VPC, clusters, etc) for community modules.
3. Everyone is amazed when PRs remove hundreds or thousands of lines of code.

:::warning New Problems
1. How will they keep their modules current and ensure they meet certain policies for the organization?
2. Which of the XYZ modules should they pick? For example, there are dozens of VPC modules. What if 2 different teams pick different ones?
3. The quality of open-source modules varies drastically. 
   - What if the modules are abandoned? 
   - What if they aren't updated to support the latest versions? 
   - What if they don't accept pull requests?
   - What if two modules conflict with each other?
:::

### **Stage 5:** Multiple Root Modules

1. Recognition of the pitfalls of having all environments in one root module and after having refactored their code heavily into modules, Developers realize it was a bad idea to define dev, staging and prod, all in the same root module. Developers realize terraform gets very slow and brittle when a root module is too large. So they break it all out, add more feature flags, and now need to manage more configuration (e.g. `.tfvars`)
2. Transition to multiple root modules, each tailored to specific environments.

:::warning New Problems
1. Increased feature flags and configuration management using `.tfvars`.
2. Performance becomes a concern with overly large root modules.
3. Very poor reusability across the organization due to the "snowflake" nature of the root modules.
:::

### **Stage 6:** Refactoring for DRY Configuration

1. Now, with many environments, many root modules, and many accounts or organizations, the problem is not how we define the infrastructure as code, it's how do we maintain all this configuration?!
2. Symlinks are used to link common files, or other similar techniques.
3. Code Generation is adopted to overcome perceived limitations of Terraform (when, in fact it's a flaw in the architecture of the project).

:::warning New Problems
1. Every time a new application is developed, the knee-jerk reaction is to write more root modules, each one bespoke to the need. Sure, each one reuses child modules, but why write a new root module for every application?
2. With more root modules, dependency management becomes difficult and there's no automatic dependency resolution for multiple root modules in Terraform.
3. Large root modules become more complicated, often necessitating the use of `-target` as a hack to work around cycles.
:::

### **Stage 7:** Terraform Scripting

1. The first path usually involves adopting a simple `bash` script or `Makefile` to orchestrate Terraform. This works well, up to a point (we know because this is how we started).
2. The scripts evolve to solve specific problems encountered that made Terraform cumbersome as a tool.

:::warning New Problems
1. What happens in practice is every team/company using Terraform ends up building different scripts, often then combining those with Makefiles, into a hodgepodge of configurations with different levels of validation. The scripts grow in complexity and have to survive generations of developers.
2. New patterns emerge, for example, hacks that involve Jinja templates, string concatenation, symlinks, and worse... sed & awk replacements!!!
3. Terraform is run mostly from local workstations/laptops with no thought for how it will run in CI/CD.
:::

### **Stage 8:** Team Adoption

1. Change velocity increases dramatically.
2. Codebase increases in size by 10x (but lots of duplication).
3. New SLA/Commitment to keep it all running all the time.

:::warning New Problems
1. Configuration starts drifting more often, as the team neglects to apply changes consistently, or "ClickOps" persists in parts of the organization.
2. Developers are stepping on each other's toes. What worked when there were only a few developers, no longer scales.
3. Poor controls and lack of consistency of Terraform code. Tons of duplication.
:::

### **Stage 9:** DIY GitOps, *Hello Jenkins!*

With the greater adoption of Terraform and DevOps principles, Developers are now using Terraform daily. They decide to use the same patterns for deploying applications with Terraform. Only Terraform is extremely different from deploying containerized apps. There are no rollbacks. It's more akin to performing database migrations without transactions (YOLO!). It's a scary business. Controls are needed.

1. Developers stick their scripts in a CI/CD pipeline. 
2. Pipeline posts comments for each commit on every PR containing the raw output of the `terraform plan`, to validate what *should* happen during `terraform apply`.
3. On merge to main, `terraform apply` is run *without* a planfile. 🤞

:::warning New Problems
- Still using Jenkins. 🧌🔥
- CI/CD system promoted to *God Mode*. 🤞 Static administrative cloud credentials are exposed as environment variables, ripe for exfiltration
- No visibility into the impact of the changes across multiple environments at the same time
- Inadequate security mechanisms, creating a critical risk of catastrophic business loss
- Lack of plan file storage means incorrect implementation of plan/apply workflows
- Missing project-level locks, so PRs can easily clobber each other, rolling back state.
- Entire organization is spammed by GitHub comments every time someone pushes a commit and a plan is run on the PR
- No recourse for when a `terraform apply` fails
- Automated drift detection needed to ensure environments converge with what's in version control
:::

### **Stage 10:** Terraform Bankruptcy 💰

Developers realize something is not right. They begin to ask, why is this so hard? 

:::danger
- Scripts all over the place that call terraform
- Inconsistent conventions on what is a module, root module and when to combine them
- Root modules that need to be provisioned in a specific order to bring them up and down, often necessitating the use of `-target`
- No way to define dependencies between root modules
- Passing state between root modules is inconsistent
- Automating the terraform in CI/CD is oversimplified, lacking project-level locks and planfile retrieval
- Inconsistent conventions or a total lack of conventions
- Incomplete or out of date documentation
- **No one understands how it all works anymore ⚠️**
:::

This should be a solved problem. 

**If only there were a tool to compose everything together...**

*(...and then they discover [Atmos](/)!)* 😉

### **Stage 11:** New Toolchain Selection

Having learned Terraform the hard way, developers are now equipped with strong opinions on what they need to win at this stage of the game.

1. **Consistent Invocation:** I want a tool that standardizes how Terraform is invoked across the organization.
2. **Made Composition Easier:** It would be amazing if it were an equivalent to Docker Compose for piecing together root modules.
3. **Efficient Configuration:** I need a method similar to Kustomize for configurations. It should reduce redundancy and support features like imports, hierarchy, inheritance, and deep merging to help set organizational standards, security, and tags with ease.
4. **Reusable Building Blocks:** I want to establish a library filled with thoroughly tested modules and components for the entire organization. After it's set up, we should hardly require additional HCL development.
5. **State Backends:** I need something to configure the state backend since Terraform doesn't support interpolations. Root modules cannot effectively be re-used without backend generation.
6. **Policy Enforcement:** I expect a mechanism to enforce specific policies and rules across configurations to control everything
7. **Structured Modularity:** I desire Terraform configurations to have a modular design to promote code reuse across varied use cases.
8. **Seamless Initialization:** I want the backend setup, module fetching, and all preparatory measures to be in place seamlessly before even thinking of using `terraform apply`.
9. **Dependency Coordination:** I'm looking for a way to ensure Terraform modules, which are dependent on one another, are applied in sequence and without hiccups.
10. **Stack Organization:** I need a system that categorizes and manages Terraform configurations using a stack-driven approach, making it easier to deal with multiple configurations.
11. **Standardized Workflows:** I would love a unified system, something in the league of `Make` or `Gotask`, for laying out workflows. This should automate repetitive tasks and be versatile enough to adapt, whether it's GitHub Actions or local setups.
12. **Environment Consistency:** It'd be great to have a tool that mirrors the consistency of `helmfile` for environment definitions, ensuring steadiness and reliability across varying environments.
13. **Turnkey CI/CD:** I want to effortlessly integrate with GitHub Actions and run operations without being charged a deployment tax.

*Oh, one last ask. Can it also be free and open source?*

## What's the Solution? *Hello Atmos!* 👽

😎 Good news! Atmos supports all of this out of the box and exactly what you **cannot** achieve with "standalone" (a.k.a community edition) Terraform and [none of the alternatives](/reference/alternatives) can do it all.

Here's what you can expect after adopting Atmos:

- **Consistent Invocation:** Atmos ensures Terraform is invoked the same way every time; no more need to pass complicated arguments to `terraform`. This removes guesswork and confusion across teams with what parameters to pass, and what order of operations things need to be invoked.
- **Seamless Initialization:** Backend? Modules? Prep steps? Atmos makes sure everything is ready before you run `terraform apply`.
- **Separation of Configuration from Code** Unlike most alternatives, Atmos clearly separates the configuration into Stacks, separate from Components (terraform root modules). This ensure root modules remain highly reusable across teams and promotes testing.
- **Enables Efficient Configuration:** Borrowing from the Kustomize playbook, Atmos makes configurations streamlined. It employs imports, hierarchy, inheritance, and deep merging, thus establishing company-wide standards effortlessly.
- **Environment Consistency:** With a nod to helmfile, Atmos ensures environments remain consistent, providing a toolkit that ensures reliability, no matter where you deploy.
- **Ensures Structured Modularity:** With Atmos, Terraform configurations are built modularly using stacks and catalogs. This not only ensures code is reusable but also that it's efficient.
- **Promotes Reusable Building Blocks:** Set up a robust library of proven and easily reusable components (terraform root modules). Once done, there's minimal need for extra HCL work. This reduces the number of custom root modules needed in an organization and facilitates integration testing.
- **Makes Composition Easier:** Think Docker Compose, but for Terraform. Atmos seamlessly integrates root modules into a stack manifest.
- **Stack Organization:** Keeping Terraform configurations organized is a breeze with Atmos's stack-based approach for combining components to build your "Stack". It's the solution for those multi-config puzzles.
- **Simplifies State Backends:** Atmos can generate a backend configuration dynamically based on the context within its stack.
- **OPA Policy Controls:** Atmos integrates [OPA](https://www.openpolicyagent.org/), offering high-level policy enforcement based on configuration values rather than the intricate HCL code details. This enhancement doesn't negate using tools like tfsec, tflint, or checkov, since none function at this elevated capacity.
- **Dependency Coordination:** Atmos takes the lead, ensuring Terraform components that depend on each other play nicely.
- **Standardized Workflows:** Workflows capture the intricate details of how to invoke commands, so it's easy to codify routine operations, the same way developers use Makefiles (or more accurately, "Gotask")
- **Turnkey CI/CD:** Integrate with [GitHub Actions](https://atmos.tools/integrations/github-actions), [Spacelift](https://atmos.tools/integrations/spacelift), or [Atlantis](https://atmos.tools/integrations/atlantis) seamlessly. With Atmos, you can avoid the steep costs associated with commercial solutions that charge based on the number of resources under management, self-hosted runners, or users.

Oh yes, and it's entirely free and truly Open Source (licensed APACHE2) like [everything else Cloud Posse builds](https://github.com/cloudposse)! 🔥
