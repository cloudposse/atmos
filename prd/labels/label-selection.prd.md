# Product Requirements Document (PRD): Label-Based Stack and Component Selection in atmos

Overview

This document proposes a scoped enhancement to the atmos CLI tool to support label-based selection of stacks and components within the list and describe command families (e.g., list vars, list components, describe component, describe stack). Inspired by Kubernetes label selectors, the goal is to allow users to filter output based on user-defined metadata labels.

Motivation

Currently, atmos list and describe commands rely on explicit filters like stack/component names, which can be verbose and inflexible. Label selectors offer a powerful, declarative way to target resources, especially in large-scale infrastructures with consistent tagging.

Goals
• Add support for label selectors (--selector or -l) to atmos list and describe commands.
• Allow labels to be defined in the metadata.labels section of both stacks and components.
• Match components within stacks and stacks themselves based on label selectors.
• Components will inherit their stack's labels during label evaluation. (Note: although metadata is not typically inherited in atmos, this behavior is explicit for label selection.)
• Integrate label-based filtering with existing filter flags (--stack, --component, --type) as described in the official docs.
• Support the following selector operators:
• Equality (key=value)
• Inequality (key!=value)
• Set-based inclusion (key in (a,b))
• Set-based exclusion (key notin (a,b))
• Existence (key)
• Non-existence (!key)
• Update all relevant documentation pages on <https://atmos.tools> related to list and describe commands with usage examples, caveats, and Kubernetes references.
• Ensure thorough test coverage

Out of Scope
• Support for label selectors in terraform or other non-list/describe commands
• Regex or partial matching
• Nested expressions or arbitrary logical combinations beyond the supported operators

Detailed Requirements

CLI Interface

Add a --selector (alias: -l) flag to relevant list and describe commands:

atmos list vars --selector 'env=prod,tier=frontend'
atmos list components --selector 'team in (platform,security)'
atmos describe component --selector 'tier=backend'
atmos describe stack --selector 'region=us-west-2'

If used in combination with --stack, the label selector will filter components within that stack. If --stack is not provided, selector applies across all stacks.

Flag Precedence Rules

All filter flags, including --selector, --stack, --component, and --type, are additive and narrowing. This means:
• Each additional filter reduces the set of results — selectors are applied in conjunction.
• If the resulting set of matches is empty, return a clear "No resources matched selector." message and exit 0.
• There is no override or precedence — filters must all match for an item to be included.

Metadata Structure

Labels will be defined under metadata.labels:

Component Example:

components:
terraform:
vpc:
metadata:
labels:
env: prod
tier: backend

Stack Example:

metadata:
labels:
region: us-east-1
team: platform

Important Note:

When evaluating label selectors for a component, it will inherit all metadata.labels from its parent stack. Component labels take precedence if the same key is defined in both places.

Matching Semantics

Match stacks/components only if all selector criteria are satisfied (logical AND), following Kubernetes semantics:
<https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors>

Error Handling
• If no matching items are found, return a message like "No resources matched selector." and exit 0.
• If the selector is invalid, return a syntax error and exit with non-zero status.

Test Plan

Add the following sample test cases to validate functionality:

Add tests for:
• Valid selectors with each operator (=, !=, in, notin, key, !key)
• Combined selectors
• Invalid syntax
• Filters that produce zero results
• Interaction with --stack, --component, and --type
• Inheritance of stack labels by components
• Label filtering in:
• list vars
• list components
• list stacks
• describe component
• describe stack

Sample Test Cases 1. Single label match at stack level:

atmos list stacks --selector 'region=us-east-1'

## Should return stacks with metadata.labels.region = us-east-1

    2.	Single label match at component level:

atmos describe component --selector 'tier=frontend'

## Should return all components labeled with tier=frontend (including inherited stack labels if applicable)

    3.	Multiple selectors across stack and component:

atmos list components --stack tenant-prod --selector 'env=prod,tier=backend'

## Should return components in tenant-prod where both env and tier match (stack labels inherited)

    4.	Component label overrides inherited stack label:

## Stack metadata:

metadata:
labels:
env: staging

## Component metadata:

metadata:
labels:
env: prod

atmos describe component --selector 'env=prod'

## Should include the component because its own env=prod overrides the stack's env=staging

    5.	Set-based matching:

atmos list vars --selector 'tier in (frontend,api)'

# #Should return vars from any stack/component where tier is frontend or api

    6.	Negation matching:

atmos list stacks --selector 'env!=dev'

# #Should return all stacks where env is not dev

    7.	No matches:

atmos describe stack --selector 'nonexistent=label'

##Should return "No resources matched selector." and exit 0

    8.	Invalid syntax:

atmos list vars --selector 'tier=('frontend')'

## Should return a syntax error and non-zero exit

Documentation Updates

Update all documentation pages related to list and describe commands at <https://atmos.tools>, including but not limited to:
• list vars
• list components
• list stacks
• describe component
• describe stack

Include:
• How to define labels in YAML metadata for both stacks and components
• The inheritance rule for labels
• How to use --selector with examples
• Supported selector syntax (linking to the Kubernetes label selector documentation)
• Common use cases and patterns

Label Selector Examples:

## List vars from all prod stacks

atmos list vars --selector 'env=prod'

## List components in us-west-2 region

atmos list components --selector 'region=us-west-2'

## Describe components with tier=backend

atmos describe component --selector 'tier=backend'

## Describe all stacks labeled for the platform team

atmos describe stack --selector 'team=platform'

Implementation Notes
• Reuse or adapt a Kubernetes-compatible label selector parser
• Integrate label evaluation into the existing filtering logic of list and describe commands
• Stack/component metadata should already be loaded during filtering phase, enabling evaluation of metadata.labels
• During filtering, merge component-level and stack-level metadata.labels for selector evaluation

Acceptance Criteria
• Users can define metadata.labels in stacks and components
• Components inherit stack labels for selector evaluation
• Label-based filtering is supported in list and describe commands via --selector
• All filters are additive and narrowing — no matches result in a clean message and zero exit
• Documentation is updated across all relevant list and describe command pages on https://atmos.tools
• All relevant tests pass

References
• Kubernetes Label Selectors: <https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors>
• Atmos CLI: <https://atmos.tools/cli/commands/>
• YAML Metadata: <https://atmos.tools/components/terraform/metadata/>
