# bastion

## Get Source AMI Programmatically via AWS CLI

```shell
aws ec2 describe-images \
  --region us-east-2 \
  --owners amazon \
  --filters 'Name=name,Values=al2023-ami-*.x86_64' \
  --query 'reverse(sort_by(Images, &CreationDate))[:1].ImageId' \
  --output text
```
