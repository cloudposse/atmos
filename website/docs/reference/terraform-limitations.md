---
title: Overcoming Terraform Limitations with Atmos
description: Overcoming Terraform Limitations with Atmos
sidebar_label: Overcoming Terraform Limitations
sidebar_position: 6
---

# Overcoming Terraform Limitations with Atmos

To better understand the rationale behind Atmos's design, it may be helpful to hear about our experiences with Terraform—a tool integral to our development processes. Let's explore the challenges and constraints we faced using Terraform, which paved the way for creating Atmos.

## What is Terraform?

Terraform is a command-line utility that processes infrastructure configurations in ["HashiCorp's Configuration Language" ("HCL")](https://en.wikipedia.org/wiki/HCL) to orchestrate infrastructure provisioning. Its chief role is to delineate and structure infrastructure definitions.
  
Terraform's HCL started strictly as a configuration language, not a markup or programming language, although has evolved considerably over the years. HCL is backward compatible with JSON, although it's not a strict superset of JSON. HCL is more human-friendly and readable, while JSON is often used for machine-generated configurations. This means you can write Terraform configurations in HCL or JSON, and Terraform will understand them. This feature is particularly useful for generating configurations programmatically or integration with systems that already use JSON.

## How has Terraform HCL Evolved?

As Terraform progressed and HCL evolved, notably from version _0.12_ onwards, HCL began incorporating fetatures typical of programming languages (albeit without a debugger!). This shift enriched infrastructure definitions, positioning HCL more as a [domain-specific programming language](https://en.wikipedia.org/wiki/Domain-specific_language) for defining infrastructure than strictly a configuration language (aka data interchange formats like JSON). As a result, the complexity of configuring Terraform projects has risen, while Terraform's inherent capabilities to be configured haven't evolved at the same pace.

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

At this initial stage, developers begin their Terraform journey with the basics, focusing on getting a grasp of its core concepts through a straightforward implementation.

1. Developers roll up their sleeves to get a simple terraform example up and running. They create resources directly in the
   configuration without the use of modules. This phase is characterized by hard-coded settings and a hands-on approach to learning
   Terraform's syntax and capabilities.
2. Local state files (since this is just an exploration). This approach simplifies the learning process by avoiding the complexities of remote state management.
3. Version control systems are not yet in use. Developers store their Terraform configurations directly on their local workstations
   allowing them to focus on learning Terraform's mechanics without the added complexity of collaboration tools or best practices.

:::warning New Problems
1. How do we handle secrets?
2. Where do we store the state file?
3. How do we maintain something with so many hardcoded settings?
:::

### **Stage 2:** The Monolithic Root Module (aka Terralith)

As developers grow more comfortable with Terraform, they often transition to a more ambitious approach of automating everything.
It results in creating a monolithic root module, also known as a Terralith. This stage is characterized by an expansive,
all-encompassing Terraform configuration that attempts to manage every aspect of the infrastructure within a single module.

1. Developers begin by composing a single root module that continuously expands.
2. Define all environments as code (dev, staging, production) in a single Terraform root module.
3. Extensive use of feature flags for everything (e.g. `prod_cluster_size`, `staging_cluster_size`, etc.)

:::warning New Problems
1. Massive blast radius for every change. It's literally scary to apply the changes, because anything can go wrong.
2. Brittle and prolonged plan/apply cycles, often disrupted by transient errors, expiring credentials and API rate limits, require frequent, manual retries.
3. Large root modules become more complicated, often necessitating the use of targeted applies (e.g. `terraform apply -target`) as a workaround for cycles.
4. Far from DRY. Significant code duplication. Tons parameters are used for toggling settings for each environment within the same root module.
5. No practical way to separate responsibilities by team within the root module.
6. Testing every parameter combination is impossible.
:::

### **Stage 3:** Move Towards Modularization

As developers dive deeper into Terraform's capabilities, they begin to recognize the inefficiencies of their initial approaches. The move towards modularization represents a significant leap in managing infrastructure as code more effectively.

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

Developers begin their Terraform journey often by crafting their own modules, initially overlooking the extensive ecosystem of pre-existing modules, such as [Cloud Posse's terraform modules](https://github.com/cloudposse).

1. Developers, initially crafting bespoke modules, discover hundreds of freely available open-source Terraform modules.
2. With the recognition of high-quality, well-maintained open-source modules, developers start to replace their custom solutions (like VPCs and clusters) with those from the community, streamlining their infrastructure code.
3. The switch to open-source modules often leads to pull requests that dramatically reduce the complexity and lines of code,
   sometimes cutting out hundreds or even thousands of lines, to the team's astonishment and delight.

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

Developers recognize the pitfalls of having all environments in one root module. After having refactored their code heavily into modules, Developers realize it was a bad idea to define dev, staging and prod, all in the same root module. Developers realize terraform gets very slow and brittle when a root module is too large.

1. Initiate the split of the monolithic root module into smaller, more manageable units while incorporating additional feature flags
   and expanding configuration management through `.tfvars` files.
2. Move towards a structure where multiple root modules are used, each one precisely aligned with specific environments
   to enhance maintainability and performance.
3. Recognize the efficiency gains and increased adaptability that came from segregating large root modules, leading to quicker
   Terraform operations and easier adjustments per environment.


:::warning New Problems
1. Increased feature flags and configuration management using `.tfvars`.
2. Performance becomes a concern with overly large root modules.
3. Very poor reusability across the organization due to the "snowflake" nature of the root modules.
:::

### **Stage 6:** Refactoring for DRY Configuration

As developers navigate the complexities of managing multiple environments, root modules, and accounts or organizations, the focus shifts from merely defining
infrastructure as code to the overarching challenge of maintaining these expansive configurations efficiently.

1. With numerous environments, root modules, and accounts or organizations, the challenge shifts from defining infrastructure as code to maintaining the extensive configuration of parameters that get passed to Terraform.
2. In an effort to manage repetitive configurations, developers resort to using symlinks or other methods to link common files across projects, seeking to reduce redundancy.
3. Code Generation is adopted to overcome *perceived* limitations of Terraform (when, in fact it's often a flaw in the architecture of the project).

:::warning New Problems
1. For every new application developed, the automatic response is to create bespoke root modules for specific needs, despite
   reusing child modules, raising the question of why a new root module is necessary for each application in the first place.
2. As the number of root modules grows, the Terraform state gets divided by component. Managing these inter-component dependencies
   falls outside Terraform's capabilities and needs to be solved in another way.
3. The adoption of code generation tools to address Terraform's *perceived* limitations (e.g., [inability to](https://github.com/hashicorp/terraform/issues/19932#issuecomment-1817043906) iterate over providers](https://github.com/hashicorp/terraform/issues/19932#issuecomment-1817043906)) that often can mask underlying architectural issues as well as make automated testing complicated.
:::

### **Stage 7:** Terraform Scripting

As Terraform projects grow in complexity and scale, developers seek ways to automate and streamline their workflows beyond what native Terraform commands offer. This leads to exploring scripting as a means to orchestrate Terraform operations.

1. The first path usually involves adopting a simple `bash` script or `Makefile` to orchestrate Terraform. This works well, up to a point (we know this because it is how we started using Terraform at Cloud Posse).
2. These scripts evolved to solve specific problems that made Terraform cumbersome as a tool.
3. Terraform scripts still run on local machines, reflecting the initial stages of development focus.

:::warning New Problems
1. What happens in practice is every team/company using Terraform ends up building different homegrown scripts, often then combining those with `Makefiles`, into a hodgepodge of configurations with different levels of validation. The scripts grow in complexity and have to survive generations of developers.
2. New patterns emerge, for example, hacks that involve Jinja templates, string concatenation, symlinks, and worse... sed & awk replacements!!!
3. Terraform is run mostly from local workstations/laptops, with no thought for how it will run in CI/CD.
:::

### **Stage 8:** Team Adoption

As Terraform use grows within the team, it becomes clear that what facilitated the initial success, is insufficient for the next level of growth and teamwork. This stage is a turning point, emphasizing the need for evolved workflows, enhanced collaboration tools, and a
more structured approach to managing scalable infrastructure projects.

1. Change velocity increases dramatically.
2. Codebase increases in size by 10x (but with lots of duplication).
3. New SLA/Commitment to keep it all running all the time.

:::warning New Problems
1. Configuration starts drifting more often as the team neglects to apply changes consistently, or "ClickOps" persists in parts of the organization.
2. Developers are stepping on each other's toes. What worked when there were only a few developers no longer scales.
3. Poor controls and lack of consistency of Terraform code. Tons of duplication.
:::

### **Stage 9:** DIY GitOps, *Hello Jenkins!*

With the greater adoption of Terraform and DevOps principles, Developers are now using Terraform daily. They decide to use the same patterns for deploying applications with Terraform. Only Terraform is exceptionally different from deploying containerized apps. There are no rollbacks. It's more akin to performing database migrations without transactions (YOLO!). It's a scary business. Controls are needed.

1. Developers stick their scripts in a CI/CD pipeline. 
2. Pipeline posts comments for each commit on every PR containing the raw output of the `terraform plan`, to validate what *should* happen during `terraform apply`.
3. On merge to main, `terraform apply` is run *without* a planfile. 🤞

:::warning New Problems
- Still using Jenkins. 🧌🔥
- CI/CD system promoted to *God Mode*. 🤞 Static administrative cloud credentials are exposed as environment variables, ripe for exfiltration
- No visibility into the impact of the changes across multiple environments at the same time
- Inadequate security mechanisms creating a critical risk of catastrophic business loss
- Lack of plan file storage means incorrect implementation of plan/apply workflows
- Missing project-level locks, so PRs can easily clobber each other, rolling back state.
- Entire organization is spammed by GitHub comments every time someone pushes a commit, and a plan is run on the PR
- No recourse for when a `terraform apply` fails
- Automated drift detection is needed to ensure environments converge with what's in version control
:::

### **Stage 10:** Terraform Bankruptcy 💥

As the complexity and scale of Terraform projects reach a tipping point, developers face a stark realization.
Questions arise about the inherent difficulties of managing sprawling infrastructure code bases using Terraform,
leading to a moment of reckoning. Some might question if Terraform is even the right tool.

:::danger

- Scripts all over the place that call terraform
- Inconsistent conventions on what is a module, root module and when to combine them
- Root modules that need to be provisioned in a specific order to bring them up and down, often necessitating the use of `-target`
- No way to define dependencies between root modules
- Passing state between root modules is inconsistent
- Automating the terraform in CI/CD is oversimplified, lacking project-level locks and `planfile` retrieval
- Inconsistent conventions or a total lack of conventions
- Incomplete or out-of-date documentation
- **No one understands how it all works anymore ⚠️**

:::

This should be a solved problem. 

**If only there were a tool to compose everything together...**

*(...and then they discover [Atmos](/)!)* 😉

### **Stage 11:** New Toolchain Selection

Having learned Terraform the hard way, developers emerge with a well-defined set of requirements for a toolchain that can address the complexities they've encountered. Their experiences have crystallized what is necessary to operate infrastructure at scale based on real-world experience.

1. **Consistent Invocation:** I want a tool that standardizes how Terraform is invoked across the organization.
2. **Made Composition Easier:** It would be fantastic if it were an equivalent to [Docker Compose](https://docs.docker.com/compose/compose-application-model/#illustrative-example) for piecing together root modules.
3. **Efficient Configuration:** I need a method similar to [Kustomize](https://kustomize.io/) for configurations. It should reduce redundancy and support features like imports, hierarchy, inheritance, and deep merging to help set organizational standards, security, and tags with ease.
4. **Reusable Building Blocks:** I want to establish a library filled with thoroughly tested modules and components for the entire organization. After it's set up, we should hardly require additional HCL development.
5. **State Backends:** I need something to configure the state backend since Terraform doesn't support interpolations. Root modules cannot effectively be re-used without backend generation.
6. **Policy Enforcement:** I expect a mechanism to enforce specific policies and rules across configurations to control everything
7. **Structured Modularity:** I desire Terraform configurations to have a modular design to promote code reuse across varied use cases.
8. **Seamless Initialization:** I want the backend setup, module fetching, and all preparatory measures to be in place seamlessly before even thinking of using `terraform apply`.
9. **Dependency Coordination:** I'm looking for a way to ensure Terraform modules, which are dependent on one another, are applied in
   sequence and without hiccups.
10. **Stack Organization:** I need a system that categorizes and manages Terraform configurations using a stack-driven approach, making it easier to deal with multiple configurations.
11. **Standardized Workflows:** I would love a unified system, something in the league of `Make` or `Gotask`, for laying out workflows. This should automate repetitive tasks and be versatile enough to adapt, whether it's GitHub Actions or local setups.
12. **Environment Consistency:** It'd be great to have a tool that mirrors the consistency of `helmfile` for environment definitions, ensuring steadiness and reliability across varying environments.
13. **Turnkey CI/CD:** I want to effortlessly integrate with GitHub Actions and run operations without being charged a deployment tax.

*Oh, one last ask. Can it also be free and open source?*

## What's the Solution? *Hello Atmos!* 👽

😎 Good news! Atmos supports all of this out of the box and exactly what you **cannot** achieve with "standalone" (a.k.a. community edition) Terraform and [none of the alternatives](/reference/alternatives) can do it all. Plus, there's no need to abandon Terraform—it's actually a great tool, and Atmos enhances its strengths.

Here's what you can expect after adopting Atmos:

- **Consistent Invocation:** Atmos ensures Terraform invokes the same way every time; no more need to pass complicated arguments to `terraform`. This removes guesswork and confusion across teams with what parameters to pass and what order of operations things need to be invoked.
- **Seamless Initialization:** Backend? Modules? Prep steps? Atmos makes sure everything is ready before you run `terraform apply`.
- **Separation of Configuration from Code** Unlike most alternatives, Atmos cleanly separates the configuration into Stacks, separate from Components (terraform root modules). This ensure root modules remain highly reusable across teams and promotes testing.
- **Enables Efficient Configuration:** Borrowing from the Kustomize playbook, Atmos makes configurations streamlined. It employs imports, hierarchy, inheritance, and deep merging, thus establishing company-wide standards effortlessly.
- **Environment Consistency:** With a nod to helmfile, Atmos ensures environments remain consistent, providing a toolkit that ensures reliability, no matter where you deploy.
- **Ensures Structured Modularity:** With Atmos, write Terraform configurations modularly using stacks and catalogs.
  This not only ensures the code is reusable but also that it's efficient.
- **Promotes Reusable Building Blocks:** Set up a robust library of proven and easily reusable components (terraform root modules). Once done, there's minimal need for extra HCL work. This reduces the number of custom root modules needed in an organization and facilitates integration testing.
- **Makes Composition Easier:** Think [Docker Compose](https://docs.docker.com/compose/intro/features-uses/), but for Terraform. Atmos seamlessly integrates root modules into a stack manifest.
- **Stack Organization:** Keeping Terraform configurations organized is a breeze with Atmos's stack-based approach for combining components to build your "Stack". It's the solution for those multi-config puzzles.
- **Simplifies State Backends:** Atmos can generate a backend configuration dynamically based on the context within its stack.
- **OPA Policy Controls:** Atmos integrates [OPA](https://www.openpolicyagent.org/), offering high-level policy enforcement based on configuration values rather than the intricate HCL code details. This enhancement doesn't negate using tools like `tfsec`, `tflint`, or `checkov`, since none function at this elevated capacity.
- **Dependency Coordination:** Atmos takes the lead, ensuring Terraform components that depend on each other play nicely.
- **Standardized Workflows:** Workflows capture the intricate details of how to invoke commands, so it's easy to codify routine operations, the same way developers use `Makefiles` (or, more recently, [`Gotask`](https://github.com/go-task/task))
- **Turnkey CI/CD:** Integrate with [GitHub Actions](https://atmos.tools/integrations/github-actions), [Spacelift](https://atmos.tools/integrations/spacelift), or [Atlantis](https://atmos.tools/integrations/atlantis) seamlessly. With Atmos, you can avoid the steep costs associated with commercial solutions that charge based on the number of resources under management, self-hosted runners, or users.

Oh yes, and it's entirely free and truly Open Source (licensed APACHE2) like [everything else Cloud Posse builds](https://github.com/cloudposse)! 🔥
