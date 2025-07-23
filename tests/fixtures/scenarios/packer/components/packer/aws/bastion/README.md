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
==> amazon-ebs.al2023: Prevalidating AMI Name: bastion-al2023-1753281768
==> amazon-ebs.al2023: Found Image ID: ami-0013ceeff668b979b
==> amazon-ebs.al2023: Setting public IP address to true on instance without a subnet ID
==> amazon-ebs.al2023: No VPC ID provided, Packer will use the default VPC
==> amazon-ebs.al2023: Inferring subnet from the selected VPC "vpc-xxxxxxxx"
==> amazon-ebs.al2023: Set subnet as "subnet-xxxxxxxx"
==> amazon-ebs.al2023: Creating temporary keypair: packer_6880f4e9-9fe4-273a-32cf-8ff991f1105f
==> amazon-ebs.al2023: Creating temporary security group for this instance: packer_6880f4eb-420c-5624-2c62-478ffdb612d9
==> amazon-ebs.al2023: Authorizing access to port 22 from [0.0.0.0/0] in the temporary security groups...
==> amazon-ebs.al2023: Launching a source AWS instance...
==> amazon-ebs.al2023: changing public IP address config to true for instance on subnet "subnet-xxxxxxxx"
==> amazon-ebs.al2023: Instance ID: i-087f3c6dcc1d9caff
==> amazon-ebs.al2023: Waiting for instance (i-087f3c6dcc1d9caff) to become ready...
==> amazon-ebs.al2023: Using SSH communicator to connect: 3.21.227.165
==> amazon-ebs.al2023: Waiting for SSH to become available...
==> amazon-ebs.al2023: Connected to SSH!
==> amazon-ebs.al2023: Provisioning with shell script: /var/folders/rt/fqmt0tmx3fs1qfzbf3qxxq700000gn/T/packer-shell1832221732
==> amazon-ebs.al2023: Waiting for process with pid 2086 to finish.
==> amazon-ebs.al2023: Amazon Linux 2023 Kernel Livepatch repository   126 kB/s |  16 kB     00:00
==> amazon-ebs.al2023: Package jq-1.7.1-49.amzn2023.0.2.aarch64 is already installed.
==> amazon-ebs.al2023: Dependencies resolved.
==> amazon-ebs.al2023: Nothing to do.
==> amazon-ebs.al2023: Complete!
==> amazon-ebs.al2023: 17 files removed
==> amazon-ebs.al2023: Stopping the source instance...
==> amazon-ebs.al2023: Stopping instance
==> amazon-ebs.al2023: Waiting for the instance to stop...
==> amazon-ebs.al2023: Creating AMI bastion-al2023-1753281768 from instance i-087f3c6dcc1d9caff
==> amazon-ebs.al2023: Attaching run tags to AMI...
==> amazon-ebs.al2023: AMI: ami-0c2ca16b7fcac7529
==> amazon-ebs.al2023: Waiting for AMI to become ready...
==> amazon-ebs.al2023: Skipping Enable AMI deprecation...
==> amazon-ebs.al2023: Skipping Enable AMI deregistration protection...
==> amazon-ebs.al2023: Modifying attributes on AMI (ami-0c2ca16b7fcac7529)...
==> amazon-ebs.al2023: Modifying: ami org arns
==> amazon-ebs.al2023: Modifying attributes on snapshot (snap-0f47c807dd52f9317)...
==> amazon-ebs.al2023: Adding tags to AMI (ami-0c2ca16b7fcac7529)...
==> amazon-ebs.al2023: Tagging snapshot: snap-0f47c807dd52f9317
==> amazon-ebs.al2023: Creating AMI tags
==> amazon-ebs.al2023: Adding tag: "SourceAMIDescription": "Amazon Linux 2023 AMI 2023.7.20250527.1 arm64 HVM kernel-6.12"
==> amazon-ebs.al2023: Adding tag: "SourceAMIName": "al2023-ami-2023.7.20250527.1-kernel-6.12-arm64"
==> amazon-ebs.al2023: Adding tag: "SourceAMIOwnerAccountId": "137112412989"
==> amazon-ebs.al2023: Adding tag: "Stage": "nonprod"
==> amazon-ebs.al2023: Adding tag: "ScanStatus": "pending"
==> amazon-ebs.al2023: Adding tag: "SourceAMI": "ami-0013ceeff668b979b"
==> amazon-ebs.al2023: Creating snapshot tags
==> amazon-ebs.al2023: Terminating the source AWS instance...
==> amazon-ebs.al2023: Cleaning up any extra volumes...
==> amazon-ebs.al2023: No volumes to clean up, skipping
==> amazon-ebs.al2023: Deleting temporary security group...
==> amazon-ebs.al2023: Deleting temporary keypair...
==> amazon-ebs.al2023: Running post-processor:  (type manifest)
Build 'amazon-ebs.al2023' finished after 3 minutes 7 seconds.

==> Wait completed after 3 minutes 7 seconds

==> Builds finished. The artifacts of successful builds are:
--> amazon-ebs.al2023: AMIs were created:
us-east-2: ami-0c2ca16b7fcac7529
```
