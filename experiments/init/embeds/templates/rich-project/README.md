# {{ .Config.name | title }}

{{ .Config.description }}

## Scaffold Template Information

- **Author**: {{ .Config.author | default "Unknown" }}
- **Year**: {{ .Config.year | default "2024" }}
- **License**: {{ .Config.license | default "MIT" }}
- **Cloud Provider**: {{ .Config.cloud_provider | upper }}
- **Environment**: {{ .Config.environment | title }}
- **Terraform Version**: {{ .Config.terraform_version | default "latest" }}
- **Created**: {{ now | date "2006-01-02 15:04:05" }}

{{ if .Config.regions }}
## AWS Regions

This scaffold template is configured to deploy to the following AWS regions:
{{ if (kindIs "slice" .Config.regions) }}
{{ range .Config.regions }}
- {{ . | upper }}
{{ end }}
{{ else }}
{{ range (splitList "," (toString .Config.regions)) }}
- {{ trim . | upper }}
{{ end }}
{{ end }}

**Total Regions**: {{ if (kindIs "slice" .Config.regions) }}{{ len .Config.regions }}{{ else }}{{ len (splitList "," (toString .Config.regions)) }}{{ end }}
{{ end }}

{{ if .Config.enable_monitoring }}
## Monitoring

This scaffold template includes monitoring and alerting infrastructure.
{{ end }}

{{ if .Config.enable_logging }}
## Logging

This scaffold template includes centralized logging infrastructure.
{{ end }}

## Getting Started

1. Install Atmos CLI
2. Configure your cloud provider credentials
3. Run `atmos terraform plan` to see what will be deployed
4. Run `atmos terraform apply` to deploy the infrastructure

## Scaffold Template Structure

```
.
├── atmos.yaml              # Atmos configuration
├── scaffold.yaml           # Scaffold template configuration schema
├── .atmos/                 # Atmos configuration directory
│   └── scaffold.yaml       # User configuration values
├── components/             # Terraform components
├── stacks/                 # Stack configurations
└── README.md              # This file
```

## Configuration

This scaffold template uses a rich configuration system. You can modify the configuration by editing `.atmos/scaffold.yaml` or re-running the initialization process.

To update the configuration:

```bash
atmos init rich-project --update
```

## License

{{ .Config.license }}

Copyright (c) {{ .Config.year }} {{ .Config.author }}
