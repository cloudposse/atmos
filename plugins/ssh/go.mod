module github.com/cloudposse/atmos/plugins/ssh

go 1.26.0

replace github.com/cloudposse/atmos/sdk => ../../sdk

require (
	github.com/cloudposse/atmos/sdk v0.0.0-00010101000000-000000000000
	github.com/ebitengine/purego v0.11.3
	github.com/stretchr/testify v1.12.1
	golang.org/x/crypto v0.59.0
	golang.org/x/sys v0.50.0
	google.golang.org/grpc v1.83.2
	google.golang.org/protobuf v1.36.12-0.20260120151049-f2248ac996af
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/fatih/color v1.13.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/hashicorp/go-hclog v1.6.3 // indirect
	github.com/hashicorp/go-plugin v1.7.0 // indirect
	github.com/hashicorp/yamux v0.1.2 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-isatty v0.0.17 // indirect
	github.com/oklog/run v1.1.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/net v0.62.0 // indirect
	golang.org/x/text v0.44.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260526163538-3dc84a4a5aaa // indirect
)
