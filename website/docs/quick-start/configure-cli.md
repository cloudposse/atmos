---
title: Configure CLI
sidebar_position: 4
sidebar_label: Configure CLI
---

In the previous step, we've decided on the following:

- Use a monorepo to configure and provision a few Terraform components into three AWS accounts and two AWS regions
- The filesystem layout for the infrastructure monorepo
- To be able to use [Component Remote State](/core-concepts/components/remote-state), we put the `atmos.yaml` CLI config file
  into `/usr/local/etc/atmos/atmos.yaml` folder and set the ENV var `ATMOS_BASE_PATH` to point to the absolute path of the root of the repo
