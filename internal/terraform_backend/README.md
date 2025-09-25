# Extending Support for a New Terraform Backend in the `!terraform.state` YAML Function

To enable a new Terraform backend type for use with the `!terraform.state` YAML function, follow these steps:

## 1. Implement the Backend Reader Function

Define a new function following the naming convention `ReadTerraformBackend<BackendType>`. This function is responsible
for retrieving the raw Terraform state from the specified backend.

**Function signature:**

```go
  func ReadTerraformBackend<BackendType>(atmosConfig *schema.AtmosConfiguration, componentSections *map[string]any) ([]byte, error)
```

**Function behavior:**

- If the state file exists, return its contents as a `[]byte`.
- If the state file does not exist (e.g., in the case when the component has not been provisioned yet), return `nil` and
  no error.

## 2. Register the Backend Reader

In `terraform_backend_registry.go`, register your backend reader implementation by mapping it to the corresponding
backend type in the `terraformBackends` registry:

```go
  func RegisterTerraformBackends() {
    terraformBackends[cfg.BackendTypeLocal] = ReadTerraformBackendLocal
    terraformBackends[cfg.BackendTypeS3] = ReadTerraformBackendS3

    // Register your new backend implementation here
    terraformBackends[cfg.BackendType<BackendType>] = ReadTerraformBackend<BackendType>
  }
```

## 3. Update Documentation

Update the corresponding documentation at:

```text
website/docs/functions/yaml/terraform.state.mdx
```

Include usage details and any backend-specific requirements or limitations.
