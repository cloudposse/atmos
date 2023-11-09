---
title: Final Notes
sidebar_position: 8
sidebar_label: Final Notes
---

Atmos provides unlimited flexibility in defining and configuring Atmos stacks and Atmos components in the stacks.

- Terraform components can be in different sub-folders in the `components/terraform` directory. The sub-folders can be organized by type, by teams
  that are responsible for the components, by operations that are performed on the components, or by any other category

- Atmos stacks can have arbitrary names and can be located in any sub-folder in the `stacks` directory. Atmos stack filesystem layout is for people to
  better organize the stacks and make the configurations DRY. Atmos (the CLI) does not care about the filesystem layout, all it cares about is how
  to find the stack and the component in the stack by using the context variables `namespace`, `tenant`, `environment` and `stage`

- An Atmos component can have any name different from the Terraform component name. For example, two different Atmos components `vpc` and `vpc-2`
  can provide configuration for the same Terraform component `vpc`

- We can provision more than one instance of the same Terraform component (with the same or different settings) into the same environment by defining
  many Atmos components that provide configuration for the Terraform component. For example, the following config shows how to define two Atmos
  components, `vpc` and `vpc-2`, which both point to the same Terraform component `vpc`:

  ```yaml
  import:
    - mixins/region/us-east-2
    - orgs/acme/plat/dev/_defaults
    - catalog/vpc
  
  components:
    terraform:
  
      vpc:
        metadata:
          component: vpc
          inherits:
            - vpc/defaults
        vars:
          name: vpc
          ipv4_primary_cidr_block: 10.9.0.0/18
  
      vpc-2:
        metadata:
          component: vpc
          inherits:
            - vpc/defaults
        vars:
          name: vpc-2
          ipv4_primary_cidr_block: 10.10.0.0/18
  ```

  <br/>

  Then we can execute the following `atmos` commands to provision the two VPCs into the `dev` account in the `us-east-2` region:

  ```shell
  atmos terraform apply vpc -s plat-ue2-dev
  atmos terraform apply vpc-2 -s plat-ue2-dev
  ```

<br/>

All the above makes Atmos an ideal framework to organize infrastructure, to design for organizational complexity, and to provision multi-account
environments for very complex organizations.
