```yaml
workflows:
  deploy-vpc:
    description: Deploy VPC infrastructure
    steps:
      - command: terraform apply vpc -s prod
      - command: terraform apply vpn -s prod

  deploy-eks:
    description: Deploy EKS cluster
    steps:
      - command: terraform apply eks -s prod
```
