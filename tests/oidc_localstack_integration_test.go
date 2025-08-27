//go:build integration_oidc_localstack
// +build integration_oidc_localstack

package tests

import (
    "context"
    "fmt"
    "io"
    "net/http"
    "net/http/httptest"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "testing"
    "time"

    tc "github.com/testcontainers/testcontainers-go"
    "github.com/testcontainers/testcontainers-go/wait"
)

// startLocalstack starts a LocalStack community container exposing edge port.
func startLocalstack(t *testing.T) (tc.Container, string) {
    t.Helper()
    ctx := context.Background()
    req := tc.ContainerRequest{
        Image:        "localstack/localstack:latest",
        ExposedPorts: []string{"4566/tcp"},
        Env: map[string]string{
            "SERVICES": "sts,iam",
        },
        WaitingFor: wait.ForListeningPort("4566/tcp").WithStartupTimeout(120 * time.Second),
    }
    c, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{ContainerRequest: req, Started: true})
    if err != nil {
        t.Fatalf("failed to start localstack: %v", err)
    }
    t.Cleanup(func() { _ = c.Terminate(ctx) })
    host, err := c.Host(ctx)
    if err != nil { t.Fatalf("host: %v", err) }
    port, err := c.MappedPort(ctx, "4566/tcp")
    if err != nil { t.Fatalf("port: %v", err) }
    return c, fmt.Sprintf("http://%s:%s", host, port.Port())
}

// stsStubServer returns a minimal STS XML responder for AssumeRoleWithWebIdentity.
func stsStubServer(t *testing.T) *httptest.Server {
    t.Helper()
    h := http.NewServeMux()
    h.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        // Accept both POST form and query
        _ = r.ParseForm()
        action := strings.ToLower(r.Form.Get("Action"))
        if action == "" {
            action = strings.ToLower(r.URL.Query().Get("Action"))
        }
        if action != "assumerolewithwebidentity" {
            http.Error(w, "unsupported action", http.StatusBadRequest)
            return
        }
        // Return static credentials
        w.Header().Set("Content-Type", "text/xml")
        _, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>
<AssumeRoleWithWebIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <AssumeRoleWithWebIdentityResult>
    <Credentials>
      <AccessKeyId>ASIAFAKEKEY</AccessKeyId>
      <SecretAccessKey>fake_secret_key</SecretAccessKey>
      <SessionToken>fake_session_token</SessionToken>
      <Expiration>2030-01-01T00:00:00Z</Expiration>
    </Credentials>
    <SubjectFromWebIdentityToken>sub</SubjectFromWebIdentityToken>
    <Provider>stub</Provider>
  </AssumeRoleWithWebIdentityResult>
  <ResponseMetadata><RequestId>req-123</RequestId></ResponseMetadata>
</AssumeRoleWithWebIdentityResponse>`)
    })
    s := httptest.NewServer(h)
    t.Cleanup(s.Close)
    return s
}

func writeFile(t *testing.T, p, s string) {
    t.Helper()
    if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
        t.Fatalf("mkdir: %v", err)
    }
    if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
        t.Fatalf("write %s: %v", p, err)
    }
}

func TestOIDC_Login_And_Terraform_With_Localstack(t *testing.T) {
    // Pre-check: ensure 'atmos' is available
    if _, err := exec.LookPath("atmos"); err != nil {
        t.Skip("atmos binary not found in PATH; build it before running this test")
    }

    // Start LocalStack
    _, lsURL := startLocalstack(t)

    // Start STS stub for OIDC login
    stsStub := stsStubServer(t)

    // Temp HOME and workdir
    tmpHome := t.TempDir()
    workdir := t.TempDir()

    // Prepare atmos scenario in temp workdir
    // Point components.base_path to the repo fixtures.
    repoRoot, _ := os.Getwd()
    repoRoot, _ = filepath.Abs(repoRoot)
    componentsBase := filepath.Join(repoRoot, "tests", "fixtures", "components", "terraform")

    atmosYAML := fmt.Sprintf(`base_path: "./"
components:
  terraform:
    base_path: %q
stacks:
  base_path: "stacks"
  included_paths:
    - "deploy/**/*"
  name_pattern: "{stage}"
auth:
  providers:
    oidcprov:
      type: aws/oidc
      region: us-east-1
      profile: oidc-prof
  identities:
    oidc:
      provider: oidcprov
      role_arn: arn:aws:iam::000000000000:role/Dummy
      web_identity_token_file: token.jwt
logs:
  level: Debug
`, componentsBase)

    writeFile(t, filepath.Join(workdir, "atmos.yaml"), atmosYAML)

    // stacks/catalog + deploy referencing mock_caller_identity
    catalog := `components:
  terraform:
    mycomponent:
      metadata:
        component: mock_caller_identity
      vars:
        region: us-east-1
`
    writeFile(t, filepath.Join(workdir, "stacks", "catalog", "mock.yaml"), catalog)

    deploy := `vars:
  stage: nonprod
import:
  - catalog/mock
components:
  terraform:
    mycomponent: {}
`
    writeFile(t, filepath.Join(workdir, "stacks", "deploy", "nonprod.yaml"), deploy)

    // Token file
    writeFile(t, filepath.Join(workdir, "token.jwt"), "header.payload.signature")

    // Common env
    env := os.Environ()
    // Isolate HOME and AWS files
    env = append(env,
        "HOME="+tmpHome,
        "AWS_CONFIG_FILE="+filepath.Join(tmpHome, ".aws", "config"),
        "AWS_SHARED_CREDENTIALS_FILE="+filepath.Join(tmpHome, ".aws", "credentials"),
        "AWS_REGION=us-east-1",
        "AWS_DEFAULT_REGION=us-east-1",
    )

    // 1) OIDC login against STS stub
    envLogin := append(env, "AWS_STS_ENDPOINT_URL="+stsStub.URL)
    cmdLogin := exec.Command("atmos", "auth", "login", "-i", "oidc")
    cmdLogin.Dir = workdir
    cmdLogin.Env = envLogin
    out, err := cmdLogin.CombinedOutput()
    if err != nil {
        t.Fatalf("auth login failed: %v\n%s", err, string(out))
    }

    // 2) Terraform plan against LocalStack (provider uses STS GetCallerIdentity)
    // Use LocalStack STS endpoint for provider calls
    envPlan := append(env, "AWS_ENDPOINT_URL_STS="+lsURL)
    cmdPlan := exec.Command("atmos", "terraform", "plan", "mycomponent", "-s", "nonprod", "--identity", "oidc")
    cmdPlan.Dir = workdir
    cmdPlan.Env = envPlan
    pout, err := cmdPlan.CombinedOutput()
    if err != nil {
        t.Fatalf("terraform plan failed: %v\n%s", err, string(pout))
    }
    if !strings.Contains(string(pout), "account_id") {
        t.Fatalf("expected plan output to contain account_id, got:\n%s", string(pout))
    }
}
