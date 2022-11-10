---
sidebar_position: 3
title: Spacelift Integration
---

Atmos natively supports [Spacelift](https://spacelift.io). This is accomplished using the [`cloudposse/terraform-spacelift-cloud-infrastructure-automation`](https://github.com/cloudposse/terraform-spacelift-cloud-infrastructure-automation) terraform module that reads the YAML Stack configurations and produces the Spacelift resources.

Cloud Posse provides 2 terraform components that implement Spacelift support.
- https://github.com/cloudposse/terraform-aws-components/tree/master/modules/spacelift-worker-pool
- https://github.com/cloudposse/terraform-aws-components/tree/master/modules/spacelift
