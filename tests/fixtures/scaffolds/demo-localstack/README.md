# LocalStack Demo

This example demonstrates how to use Atmos with LocalStack for local AWS development.

## Prerequisites

- Docker and Docker Compose
- Atmos CLI
- AWS CLI (optional, for verification)

## Setup

1. **Start LocalStack**:
   ```bash
   docker-compose up -d
   ```

2. **Configure AWS CLI for LocalStack** (optional):
   ```bash
   aws configure set aws_access_key_id test
   aws configure set aws_secret_access_key test
   aws configure set default.region us-east-1
   aws configure set default.output json
   ```

3. **Set environment variables**:
   ```bash
   export AWS_ACCESS_KEY_ID=test
   export AWS_SECRET_ACCESS_KEY=test
   export AWS_DEFAULT_REGION=us-east-1
   export TF_VAR_aws_endpoint=http://localhost:4566
   ```

## Usage

1. **Initialize Terraform**:
   ```bash
   atmos terraform init <component> -s <stack>
   ```

2. **Plan infrastructure**:
   ```bash
   atmos terraform plan <component> -s <stack>
   ```

3. **Apply infrastructure**:
   ```bash
   atmos terraform apply <component> -s <stack>
   ```

## Benefits

- **Local Development**: Test infrastructure code without AWS costs
- **Fast Iteration**: No network latency for AWS API calls
- **Isolated Environment**: Safe testing environment
- **Cost Effective**: No AWS charges for development

## Limitations

- Not all AWS services are supported
- Some advanced features may not work exactly like AWS
- Performance may differ from real AWS

## Cleanup

```bash
# Stop LocalStack
docker-compose down

# Remove volumes (optional)
docker-compose down -v
```
