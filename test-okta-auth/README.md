# Okta Authentication Test

This directory contains a test configuration for the Okta authentication feature.

## Prerequisites

### 1. Create an Okta Application

1. Log into your Okta Admin Console
2. Go to **Applications** → **Create App Integration**
3. Select **OIDC - OpenID Connect** and **Native Application**
4. Configure the application:
   - **App name**: Atmos CLI
   - **Grant type**: Enable **Device Authorization**
   - **Assignments**: Assign users/groups that should access Atmos

5. Note down:
   - **Client ID** (e.g., `0oa1234567890abcdef`)
   - **Okta domain** (e.g., `your-company.okta.com`)

### 2. Update atmos.yaml

Edit `atmos.yaml` and replace:
- `org_url`: Your Okta org URL (e.g., `https://your-company.okta.com`)
- `client_id`: Your Okta application client ID

### 3. (Optional) AWS OIDC Federation

To use Okta for AWS authentication:

1. Create an IAM Identity Provider in AWS:
   - Type: OpenID Connect
   - Provider URL: `https://your-company.okta.com`
   - Audience: Your Okta client ID

2. Create an IAM Role that trusts the OIDC provider:
   ```json
   {
     "Version": "2012-10-17",
     "Statement": [{
       "Effect": "Allow",
       "Principal": {
         "Federated": "arn:aws:iam::123456789012:oidc-provider/your-company.okta.com"
       },
       "Action": "sts:AssumeRoleWithWebIdentity",
       "Condition": {
         "StringEquals": {
           "your-company.okta.com:aud": "your-client-id"
         }
       }
     }]
   }
   ```

3. Update `atmos.yaml` with the role ARN in `aws-via-okta` identity

## Testing

### Build Atmos

```bash
cd /Users/rosesecurity/Desktop/Projects/atmos
make build
```

### Test Okta Login

```bash
cd test-okta-auth

# Login with Okta API identity
../build/atmos auth login --identity okta-api

# This will:
# 1. Display a device code and URL
# 2. Open your browser to Okta login
# 3. After completing login, tokens are cached

# Check login status
../build/atmos auth whoami --identity okta-api
```

### Test AWS Federation (if configured)

```bash
# Login with AWS via Okta OIDC
../build/atmos auth login --identity aws-via-okta

# Check AWS credentials
../build/atmos auth whoami --identity aws-via-okta
```

### Logout

```bash
# Logout from Okta
../build/atmos auth logout --identity okta-api

# Logout from AWS (if used)
../build/atmos auth logout --identity aws-via-okta
```

## Token Storage

Tokens are stored in XDG-compliant paths:
- macOS/Linux: `~/.config/atmos/okta/{provider-name}/tokens.json`
- With realm: `~/.config/atmos/{realm}/okta/{provider-name}/tokens.json`

## Troubleshooting

### "Device code expired"
The device code expires after a few minutes. Run `atmos auth login` again.

### "Invalid client_id"
Verify the client_id in atmos.yaml matches your Okta application.

### "User denied authorization"
The user clicked "Deny" during Okta login. Run `atmos auth login` again.

### AWS federation fails
- Ensure the IAM Identity Provider is configured correctly
- Verify the role trust policy includes your Okta domain and client ID
- Check that the ID token includes required claims
