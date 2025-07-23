# bastion

## Get Source AMI Programmatically via AWS CLI

```shell
aws ec2 describe-images \
  --region us-east-2 \
  --owners amazon \
  --filters 'Name=name,Values=al2023-ami-*.arm64' \
  --query 'reverse(sort_by(Images, &CreationDate))[:1].ImageId' \
  --output text
```

## Provision

```shell
atmos packer build aws/bastion main.pkr.hcl -s nonprod
```

```console
==> amazon-ebs.al2023: Prevalidating any provided VPC information
==> amazon-ebs.al2023: Prevalidating AMI Name: bastion
==> amazon-ebs.al2023: Found Image ID: ami-0013ceeff668b979b
==> amazon-ebs.al2023: Creating temporary keypair: packer_68805b24-540d-9dcf-6f21-6282616011ef
==> amazon-ebs.al2023: Creating temporary security group for this instance: packer_68805b25-35a1-eb8d-c741-5b97c4552b4e
==> amazon-ebs.al2023: Authorizing access to port 22 from [0.0.0.0/0] in the temporary security groups...
==> amazon-ebs.al2023: Launching a source AWS instance...
==> amazon-ebs.al2023: Instance ID: i-0261a6df2e83faddb
==> amazon-ebs.al2023: Waiting for instance (i-0261a6df2e83faddb) to become ready...
==> amazon-ebs.al2023: Using SSH communicator to connect: 18.116.170.29
```
