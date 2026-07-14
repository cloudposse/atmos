module github.com/cloudposse/atmos

go 1.26.4

require (
	al.essio.dev/pkg/shellescape v1.6.0
	cloud.google.com/go/secretmanager v1.20.0
	cloud.google.com/go/storage v1.62.2
	dario.cat/mergo v1.0.2
	filippo.io/age v1.3.1
	github.com/1Password/connect-sdk-go v1.5.3
	// Pinned to v0.3.1: v0.4.0+ adds a desktop-integration build guard that fails to compile
	// under CGO_ENABLED=0 (Atmos's cross-platform build mode). Do not bump without verifying
	// `CGO_ENABLED=0 go build .` still succeeds. See pkg/store/onepassword_store.go.
	github.com/1password/onepassword-sdk-go v0.3.1
	github.com/99designs/keyring v1.2.2
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.21.1
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.13.1
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions v1.3.0
	github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets v1.4.0
	github.com/Azure/azure-sdk-for-go/sdk/storage/azblob v1.7.0
	github.com/AzureAD/microsoft-authentication-library-for-go v1.7.2
	github.com/HdrHistogram/hdrhistogram-go v1.2.0
	github.com/Masterminds/semver/v3 v3.5.0
	github.com/Masterminds/sprig/v3 v3.3.0
	github.com/PuerkitoBio/goquery v1.12.0
	github.com/adrg/xdg v0.5.3
	github.com/agiledragon/gomonkey/v2 v2.14.0
	github.com/alecthomas/chroma/v2 v2.24.1
	github.com/alicebob/miniredis/v2 v2.37.0
	github.com/anthropics/anthropic-sdk-go v1.43.0
	github.com/arsham/figurine v1.3.0
	github.com/atotto/clipboard v0.1.4
	github.com/aws/aws-sdk-go-v2 v1.42.1
	github.com/aws/aws-sdk-go-v2/config v1.32.18
	github.com/aws/aws-sdk-go-v2/credentials v1.19.17
	github.com/aws/aws-sdk-go-v2/service/bedrockruntime v1.52.0
	github.com/aws/aws-sdk-go-v2/service/ecr v1.57.2
	github.com/aws/aws-sdk-go-v2/service/ecrpublic v1.40.0
	github.com/aws/aws-sdk-go-v2/service/eks v1.84.0
	github.com/aws/aws-sdk-go-v2/service/inspector2 v1.52.0
	github.com/aws/aws-sdk-go-v2/service/organizations v1.51.3
	github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi v1.31.12
	github.com/aws/aws-sdk-go-v2/service/s3 v1.101.0
	github.com/aws/aws-sdk-go-v2/service/securityhub v1.71.0
	github.com/aws/aws-sdk-go-v2/service/ssm v1.68.6
	github.com/aws/aws-sdk-go-v2/service/sso v1.30.17
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.36.0
	github.com/aws/aws-sdk-go-v2/service/sts v1.42.1
	github.com/aws/smithy-go v1.27.3
	github.com/bmatcuk/doublestar/v4 v4.10.0
	github.com/charmbracelet/bubbles v1.0.0
	github.com/charmbracelet/bubbletea v1.3.10
	github.com/charmbracelet/colorprofile v0.4.3 // indirect
	github.com/charmbracelet/glamour v1.0.0
	github.com/charmbracelet/huh v1.0.0
	github.com/charmbracelet/lipgloss v1.1.1-0.20250404203927-76690c660834
	github.com/charmbracelet/log v1.0.0
	github.com/charmbracelet/x/ansi v0.11.7
	github.com/charmbracelet/x/term v0.2.2
	github.com/cockroachdb/errors v1.13.0
	github.com/creack/pty v1.1.24
	github.com/editorconfig-checker/editorconfig-checker/v3 v3.6.1
	github.com/elewis787/boa v0.1.3
	github.com/expr-lang/expr v1.17.8
	github.com/fatih/color v1.19.0
	github.com/gabriel-vasile/mimetype v1.4.13
	github.com/getsentry/sentry-go v0.46.2
	github.com/getsops/sops/v3 v3.13.1
	github.com/go-git/go-git/v5 v5.19.1
	github.com/go-viper/mapstructure/v2 v2.5.0
	github.com/gobwas/glob v0.2.3
	github.com/goccy/go-yaml v1.19.2
	github.com/gofrs/flock v0.13.0
	github.com/google/cel-go v0.26.0
	github.com/google/go-cmp v0.7.0
	github.com/google/go-containerregistry v0.21.7
	github.com/google/go-github/v59 v59.0.0
	github.com/google/renameio/v2 v2.0.2
	github.com/google/uuid v1.6.0
	github.com/googleapis/gax-go/v2 v2.22.0
	github.com/hairyhenderson/gomplate/v3 v3.11.8
	github.com/hairyhenderson/gomplate/v4 v4.3.3
	github.com/hashicorp/go-getter v1.8.6
	github.com/hashicorp/go-version v1.9.0
	github.com/hashicorp/golang-lru/v2 v2.0.7
	github.com/hashicorp/hcl v1.0.1-vault-7
	github.com/hashicorp/hcl/v2 v2.24.0
	github.com/hashicorp/terraform-config-inspect v0.0.0-20260224005459-813a97530220
	github.com/hashicorp/terraform-exec v0.25.2
	github.com/hashicorp/terraform-json v0.27.2
	github.com/hexops/gotextdiff v1.0.3
	github.com/jfrog/jfrog-client-go v1.55.0
	github.com/johannesboyne/gofakes3 v0.0.0-20260208201424-4c385a1f6a73
	github.com/joho/godotenv v1.5.1
	github.com/json-iterator/go v1.1.13-0.20220915233716-71ac16282d12
	github.com/jwalton/go-supportscolor v1.2.0
	github.com/kubescape/go-git-url v0.0.31
	github.com/lrstanley/bubblezone v1.0.0
	github.com/mattn/go-isatty v0.0.22
	github.com/mattn/go-runewidth v0.0.23
	github.com/mikefarah/yq/v4 v4.53.2
	github.com/mitchellh/go-wordwrap v1.0.1
	github.com/mitchellh/mapstructure v1.5.0
	github.com/modelcontextprotocol/go-sdk v1.6.1
	github.com/muesli/ansi v0.0.0-20230316100256-276c6243b2f6
	github.com/muesli/reflow v0.3.0
	github.com/muesli/termenv v0.16.0
	github.com/open-policy-agent/opa v1.16.2
	github.com/openai/openai-go v1.12.0
	github.com/opencontainers/image-spec v1.1.1
	github.com/otiai10/copy v1.14.1
	github.com/pact-foundation/pact-go/v2 v2.5.1
	github.com/pkg/errors v0.9.1
	github.com/playwright-community/playwright-go v0.5700.1
	github.com/posthog/posthog-go v1.12.6
	github.com/redis/go-redis/v9 v9.19.0
	github.com/samber/lo v1.53.0
	github.com/santhosh-tekuri/jsonschema/v5 v5.3.1
	github.com/sergi/go-diff v1.4.0
	github.com/sirupsen/logrus v1.9.4
	github.com/spf13/cobra v1.10.2
	github.com/spf13/pflag v1.0.10
	github.com/spf13/viper v1.21.0
	github.com/stretchr/testify v1.11.1
	github.com/terraform-docs/terraform-docs v0.24.0
	github.com/tliron/glsp v0.2.2
	github.com/versent/saml2aws/v2 v2.36.19
	github.com/xeipuuv/gojsonschema v1.2.0
	github.com/yuin/goldmark v1.8.2
	github.com/zalando/go-keyring v0.2.8
	github.com/zclconf/go-cty v1.18.1
	go.uber.org/mock v0.6.0
	go.yaml.in/yaml/v3 v3.0.4
	golang.org/x/oauth2 v0.36.0
	golang.org/x/sync v0.22.0
	golang.org/x/term v0.44.0
	golang.org/x/text v0.38.0
	google.golang.org/api v0.280.0
	google.golang.org/genai v1.58.0
	google.golang.org/grpc v1.81.1
	gopkg.in/ini.v1 v1.67.2
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.1
	helm.sh/helm/v4 v4.2.1
	k8s.io/client-go v0.36.2
	modernc.org/sqlite v1.50.1
	mvdan.cc/sh/v3 v3.13.1
	sigs.k8s.io/kustomize/api v0.21.1
	sigs.k8s.io/kustomize/kyaml v0.21.1
)

require (
	cel.dev/expr v0.25.2 // indirect
	cloud.google.com/go v0.123.0 // indirect
	cloud.google.com/go/auth v0.20.0 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.8 // indirect
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	cloud.google.com/go/iam v1.11.0 // indirect
	cloud.google.com/go/monitoring v1.29.0 // indirect
	cuelang.org/go v0.16.1 // indirect
	github.com/99designs/go-keychain v0.0.0-20191008050251-8e49817e8af4 // indirect
	github.com/AlecAivazis/survey/v2 v2.3.7 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.12.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/internal v1.2.0 // indirect
	github.com/Azure/go-autorest v14.2.0+incompatible // indirect
	github.com/Azure/go-autorest/autorest/to v0.4.1 // indirect
	github.com/Azure/go-ntlmssp v0.1.1 // indirect
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/CycloneDX/cyclonedx-go v0.11.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp v1.32.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric v0.56.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/internal/resourcemapping v0.56.0 // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Microsoft/go-winio v0.6.3-0.20251027160822-ad3df93bed29 // indirect
	github.com/ProtonMail/go-crypto v1.4.1 // indirect
	github.com/Shopify/ejson v1.5.4 // indirect
	github.com/a8m/envsubst v1.4.3 // indirect
	github.com/agext/levenshtein v1.2.3 // indirect
	github.com/agnivade/levenshtein v1.2.1 // indirect
	github.com/alecthomas/participle/v2 v2.1.4 // indirect
	github.com/andybalholm/brotli v1.2.1 // indirect
	github.com/andybalholm/cascadia v1.3.3 // indirect
	github.com/apparentlymart/go-cidr v1.1.1 // indirect
	github.com/apparentlymart/go-textseg/v15 v15.0.0 // indirect
	github.com/armon/go-metrics v0.4.1 // indirect
	github.com/arsham/rainbow v1.2.1 // indirect
	github.com/avast/retry-go v3.0.0+incompatible // indirect
	github.com/aws/aws-sdk-go v1.55.8 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.10 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.23 // indirect
	github.com/aws/aws-sdk-go-v2/feature/s3/manager v1.22.19 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.30 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.30 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.24 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.9 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.9.15 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.23 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.19.23 // indirect
	github.com/aws/aws-sdk-go-v2/service/secretsmanager v1.41.7
	github.com/aws/aws-sdk-go-v2/service/signin v1.0.11 // indirect
	github.com/aymanbagabas/go-osc52/v2 v2.0.1 // indirect
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/bearsh/hid v1.6.0 // indirect
	github.com/beevik/etree v1.6.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bgentry/go-netrc v0.0.0-20140422174119-9fd32a8b3d3d // indirect
	github.com/buger/jsonparser v1.2.0 // indirect
	github.com/catppuccin/go v0.3.0 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/chainguard-dev/git-urls v1.0.2 // indirect
	github.com/charmbracelet/harmonica v0.2.0 // indirect
	github.com/charmbracelet/x/cellbuf v0.0.15
	github.com/charmbracelet/x/exp/slice v0.0.0-20260525135217-abeec2b8bf0b // indirect
	github.com/charmbracelet/x/exp/strings v0.1.0 // indirect
	github.com/clipperhouse/displaywidth v0.11.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/cloudflare/circl v1.6.3 // indirect
	github.com/cncf/xds/go v0.0.0-20260202195803-dba9d589def2 // indirect
	github.com/cockroachdb/apd/v3 v3.2.3 // indirect
	github.com/cockroachdb/logtags v0.0.0-20241215232642-bb51bb14a506 // indirect
	github.com/cockroachdb/redact v1.1.8 // indirect
	github.com/common-nighthawk/go-figure v0.0.0-20210622060536-734e95fb86be // indirect
	github.com/containerd/containerd/v2 v2.3.3 // indirect
	github.com/containerd/errdefs v1.0.0 // indirect
	github.com/containerd/log v0.1.0 // indirect
	github.com/containerd/platforms v1.0.0-rc.4 // indirect
	github.com/containerd/typeurl/v2 v2.2.3 // indirect
	github.com/cyphar/filepath-securejoin v0.6.1 // indirect
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/deckarep/golang-set/v2 v2.9.0 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.1 // indirect
	github.com/dgraph-io/ristretto/v2 v2.4.0 // indirect
	github.com/dimchansky/utfbom v1.1.1 // indirect
	github.com/dlclark/regexp2 v1.12.0 // indirect
	github.com/docker/cli v29.5.3+incompatible // indirect
	github.com/docker/docker-credential-helpers v0.9.7 // indirect
	github.com/docker/libkv v0.2.2-0.20180912205406-458977154600 // indirect
	github.com/dsnet/compress v0.0.1 // indirect
	github.com/dustin/go-humanize v1.0.1
	github.com/dustin/gojson v0.0.0-20160307161227-2e71ec9dd5ad // indirect
	github.com/dvsekhvalnov/jose2go v1.8.0 // indirect
	github.com/editorconfig/editorconfig-core-go/v2 v2.6.4
	github.com/elliotchance/orderedmap v1.8.0 // indirect
	github.com/emicklei/proto v1.14.3 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/envoyproxy/go-control-plane/envoy v1.37.0 // indirect
	github.com/envoyproxy/protoc-gen-validate v1.3.3 // indirect
	github.com/erikgeiser/coninput v0.0.0-20211004153227-1c3628e74d0f // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/forPelevin/gomoji v1.4.1 // indirect
	github.com/fsnotify/fsnotify v1.10.1 // indirect
	github.com/fxamacker/cbor/v2 v2.9.2 // indirect
	github.com/go-git/gcfg v1.5.1-0.20230307220236-3a3c6141e376 // indirect
	github.com/go-git/go-billy/v5 v5.9.0
	github.com/go-ini/ini v1.67.0 // indirect
	github.com/go-jose/go-jose/v3 v3.0.5 // indirect
	github.com/go-jose/go-jose/v4 v4.1.4 // indirect
	github.com/go-logfmt/logfmt v0.6.1 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-stack/stack v1.8.1 // indirect
	github.com/goccy/go-json v0.10.6 // indirect
	github.com/godbus/dbus v4.1.0+incompatible // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang-jwt/jwt/v4 v4.5.2 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/golang/snappy v1.0.0 // indirect
	github.com/google/flatbuffers v25.12.19+incompatible // indirect
	github.com/google/go-querystring v1.2.0 // indirect
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/google/wire v0.7.0 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.16 // indirect
	github.com/gookit/color v1.6.1 // indirect
	github.com/gorilla/css v1.0.1 // indirect
	github.com/gorilla/websocket v1.5.4-0.20250319132907-e064f32e3674 // indirect
	github.com/gosimple/slug v1.15.0 // indirect
	github.com/gosimple/unidecode v1.0.1 // indirect
	github.com/gsterjov/go-libsecret v0.0.0-20161001094733-a6f4afe4910c // indirect
	github.com/hack-pad/hackpadfs v0.2.4 // indirect
	github.com/hairyhenderson/go-fsimpl v0.3.1 // indirect; Pinned: transitive via gomplate/v3; newer go-fsimpl versions require gocloud.dev > v0.41.0, which is also pinned (see gocloud.dev comment below).
	github.com/hairyhenderson/toml v0.4.2-0.20210923231440-40456b8e66cf // indirect
	github.com/hairyhenderson/xignore v0.3.3-0.20230403012150-95fe86932830 // indirect
	github.com/hairyhenderson/yaml v0.0.0-20220618171115-2d35fca545ce // indirect
	github.com/hashicorp/aws-sdk-go-base/v2 v2.0.0-beta.72 // indirect
	github.com/hashicorp/consul/api v1.34.3 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-hclog v1.6.3 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/hashicorp/go-metrics v0.5.4 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.8 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/go-secure-stdlib/awsutil v0.3.0 // indirect
	github.com/hashicorp/go-secure-stdlib/parseutil v0.2.0 // indirect
	github.com/hashicorp/go-secure-stdlib/strutil v0.1.2 // indirect
	github.com/hashicorp/go-sockaddr v1.0.7 // indirect
	github.com/hashicorp/go-uuid v1.0.3 // indirect
	github.com/hashicorp/golang-lru v1.0.2 // indirect
	github.com/hashicorp/serf v0.10.2 // indirect
	github.com/hashicorp/vault/api v1.23.0
	github.com/hashicorp/vault/api/auth/approle v0.12.0 // indirect
	github.com/hashicorp/vault/api/auth/aws v0.12.0 // indirect
	github.com/hashicorp/vault/api/auth/userpass v0.12.0 // indirect
	github.com/huandu/go-clone v1.7.3 // indirect
	github.com/huandu/go-sqlbuilder v1.41.0 // indirect
	github.com/huandu/xstrings v1.5.0 // indirect
	github.com/iancoleman/orderedmap v0.3.0 // indirect
	github.com/iancoleman/strcase v0.3.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/invopop/jsonschema v0.13.0 // indirect; Pinned: anthropic-sdk-go uses wk8/go-ordered-map/v2 for jsonschema.Schema.Properties. invopop/jsonschema v0.14.0+ switched to pb33f/ordered-map/v2, breaking type compatibility. Bump after anthropic-sdk-go migrates.
	github.com/itchyny/gojq v0.12.19 // indirect
	github.com/itchyny/timefmt-go v0.1.8 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/jfrog/archiver/v3 v3.6.3 // indirect
	github.com/jfrog/build-info-go v1.13.0 // indirect
	github.com/jfrog/gofrog v1.7.6 // indirect
	github.com/jinzhu/copier v0.4.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/kevinburke/ssh_config v1.6.0 // indirect
	github.com/klauspost/compress v1.18.6 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/klauspost/pgzip v1.2.6 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/lestrrat-go/blackmagic v1.0.4 // indirect
	github.com/lestrrat-go/dsig v1.3.0 // indirect
	github.com/lestrrat-go/dsig-secp256k1 v1.0.0 // indirect
	github.com/lestrrat-go/httpcc v1.0.1 // indirect
	github.com/lestrrat-go/httprc/v3 v3.0.5 // indirect
	github.com/lestrrat-go/jwx/v3 v3.1.1 // indirect
	github.com/lestrrat-go/option/v2 v2.0.0 // indirect
	github.com/lucasb-eyer/go-colorful v1.4.0 // indirect
	github.com/magiconair/properties v1.8.10 // indirect
	github.com/mailru/easyjson v0.9.2 // indirect
	github.com/marshallbrekka/go-u2fhost v0.0.0-20210111072507-3ccdec8c8105 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-localereader v0.0.1 // indirect
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d
	github.com/microcosm-cc/bluemonday v1.0.27 // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/hashstructure/v2 v2.0.2 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/moby/locker v1.0.1 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/mtibben/percent v0.2.1 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/nwaples/rardecode/v2 v2.2.3 // indirect
	github.com/opencontainers/go-digest v1.0.1-0.20231025023718-d50d2fec9c98 // indirect
	github.com/otiai10/mint v1.6.3 // indirect
	github.com/pelletier/go-toml/v2 v2.3.1 // indirect
	github.com/petermattis/goid v0.0.0-20260330135022-df67b199bc81 // indirect
	github.com/pierrec/lz4/v4 v4.1.26 // indirect
	github.com/pjbgf/sha1cd v0.6.0 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_golang v1.23.2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.67.5 // indirect
	github.com/prometheus/procfs v0.20.1 // indirect
	github.com/protocolbuffers/txtpbfmt v0.0.0-20260420112717-c39628bde8b5 // indirect
	github.com/rcrowley/go-metrics v0.0.0-20250401214520-65e299d6c5c9 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/rs/zerolog v1.35.1 // indirect
	github.com/ryanuber/go-glob v1.0.0 // indirect
	github.com/ryszard/goskiplist v0.0.0-20150312221310-2dfbae5fcf46 // indirect
	github.com/sagikazarmark/locafero v0.12.0 // indirect
	github.com/sahilm/fuzzy v0.1.2 // indirect
	github.com/sasha-s/go-deadlock v0.3.9 // indirect
	github.com/segmentio/asm v1.2.1 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/segmentio/ksuid v1.0.4 // indirect
	github.com/shopspring/decimal v1.4.0 // indirect
	github.com/skeema/knownhosts v1.3.2 // indirect
	github.com/sourcegraph/jsonrpc2 v0.2.1 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spiffe/go-spiffe/v2 v2.6.0 // indirect
	github.com/stretchr/objx v0.5.3 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/tchap/go-patricia/v2 v2.3.3 // indirect
	github.com/terraform-docs/terraform-config-inspect v0.0.0-20250408153412-5b88c7ed5b63 // indirect
	github.com/tidwall/gjson v1.19.0 // indirect
	github.com/tidwall/match v1.2.0 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/tliron/commonlog v0.2.21 // indirect
	github.com/tliron/go-kutil v0.4.0 // indirect
	github.com/trimble-oss/go-webauthn-client v0.3.0 // indirect
	github.com/ugorji/go/codec v1.3.1 // indirect
	github.com/ulikunitz/xz v0.5.15 // indirect
	github.com/valyala/fastjson v1.6.10 // indirect
	github.com/vektah/gqlparser/v2 v2.5.33 // indirect
	github.com/wk8/go-ordered-map/v2 v2.1.8 // indirect
	github.com/wlynxg/chardet v1.0.5 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/xanzy/ssh-agent v0.3.3 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	github.com/yashtewari/glob-intersection v0.2.0 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	github.com/yuin/goldmark-emoji v1.0.6 // indirect
	github.com/yuin/gopher-lua v1.1.2 // indirect
	github.com/zealic/xignore v0.3.3 // indirect
	go.etcd.io/bbolt v1.4.3 // indirect
	go.mongodb.org/mongo-driver v1.17.9 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/detectors/gcp v1.43.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.68.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.68.0 // indirect
	go.opentelemetry.io/otel v1.43.0 // indirect
	go.opentelemetry.io/otel/metric v1.43.0 // indirect
	go.opentelemetry.io/otel/sdk v1.43.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.43.0 // indirect
	go.opentelemetry.io/otel/trace v1.43.0 // indirect
	go.shabbyrobe.org/gocovmerge v0.0.0-20230507111327-fa4f82cfbf4d // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.yaml.in/yaml/v2 v2.4.4 // indirect
	go.yaml.in/yaml/v4 v4.0.0-rc.4 // indirect
	go4.org/intern v0.0.0-20230525184215-6c62f75575cb // indirect
	go4.org/netipx v0.0.0-20231129151722-fdeea329fbba // indirect
	go4.org/unsafe/assume-no-moving-gc v0.0.0-20231121144256-b99613f794b6 // indirect
	gocloud.dev v0.41.0 // indirect; Pinned: gomplate/v3's s3blob code references s3blob.URLOpener.ConfigProvider, which was removed in gocloud.dev v0.42+. Bump when gomplate/v3 updates or when we migrate the gomplate/v3 usages to gomplate/v4.
	golang.org/x/crypto v0.53.0
	golang.org/x/exp v0.0.0-20260508232706-74f9aab9d74a // indirect
	golang.org/x/mod v0.37.0 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/sys v0.46.0
	golang.org/x/time v0.15.0 // indirect
	golang.org/x/tools v0.46.0 // indirect
	golang.org/x/xerrors v0.0.0-20240903120638-7835f813f4da // indirect
	google.golang.org/genproto v0.0.0-20260523011958-0a33c5d7ca68 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260523011958-0a33c5d7ca68 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260523011958-0a33c5d7ca68 // indirect
	google.golang.org/protobuf v1.36.12-0.20260120151049-f2248ac996af // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
	inet.af/netaddr v0.0.0-20230525184311-b8eac61e914a // indirect
	k8s.io/apimachinery v0.36.2
	k8s.io/klog/v2 v2.140.0 // indirect
	k8s.io/kube-openapi v0.0.0-20260520065146-aa012df4f4af // indirect
	k8s.io/utils v0.0.0-20260507154919-ff6756f316d2 // indirect
	modernc.org/libc v1.72.5 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
	mvdan.cc/xurls/v2 v2.6.0 // indirect
	oras.land/oras-go/v2 v2.6.2 // indirect
	sigs.k8s.io/json v0.0.0-20250730193827-2d320260d730 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/structured-merge-diff/v6 v6.4.0 // indirect
	sigs.k8s.io/yaml v1.6.0
)

require sigs.k8s.io/controller-runtime v0.24.1

require github.com/testcontainers/testcontainers-go v0.42.0

require github.com/aws/aws-sdk-go-v2/service/kms v1.51.1

require (
	github.com/databus23/helm-diff/v3 v3.15.10
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510
	github.com/minamijoyo/hcledit v0.2.18
	github.com/mxschmitt/playwright-go v0.6100.0
	github.com/updatecli/updatecli v0.999.0
	golang.org/x/image v0.43.0
)

require (
	cloud.google.com/go/kms v1.31.0 // indirect
	cloud.google.com/go/longrunning v1.0.0 // indirect
	filippo.io/edwards25519 v1.2.0 // indirect
	filippo.io/hpke v0.4.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azkeys v1.4.0 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20250102033503-faa5f7b0171c // indirect
	github.com/MakeNowJust/heredoc v1.0.0 // indirect
	github.com/Masterminds/squirrel v1.5.4 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.0 // indirect
	github.com/aryann/difflib v0.0.0-20210328193216-ff5ff6dc229b // indirect
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2 // indirect
	github.com/blang/semver v3.5.1+incompatible // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/bytecodealliance/wasmtime-go/v43 v43.0.2 // indirect
	github.com/chai2010/gettext-go v1.0.3 // indirect
	github.com/containerd/errdefs/pkg v0.3.0 // indirect
	github.com/containerd/ttrpc v1.2.8 // indirect
	github.com/cpuguy83/dockercfg v0.3.2 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.7 // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/docker/go-connections v0.7.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/dylibso/observe-sdk/go v0.0.0-20240828172851-9145d8ad07e1 // indirect
	github.com/ebitengine/purego v0.10.0 // indirect
	github.com/emicklei/go-restful/v3 v3.13.0 // indirect
	github.com/evanphx/json-patch/v5 v5.9.11 // indirect
	github.com/exponent-io/jsonpath v0.0.0-20210407135951-1de76d718b3f // indirect
	github.com/extism/go-sdk v1.7.1 // indirect
	github.com/fluxcd/cli-utils v1.2.1 // indirect
	github.com/getsops/gopgagent v0.0.0-20241224165529-7044f28e491e // indirect
	github.com/go-errors/errors v1.5.1 // indirect
	github.com/go-gorp/gorp/v3 v3.1.0 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/go-openapi/jsonpointer v0.22.5 // indirect
	github.com/go-openapi/jsonreference v0.21.5 // indirect
	github.com/go-openapi/swag v0.25.5 // indirect
	github.com/go-openapi/swag/cmdutils v0.25.5 // indirect
	github.com/go-openapi/swag/conv v0.25.5 // indirect
	github.com/go-openapi/swag/fileutils v0.25.5 // indirect
	github.com/go-openapi/swag/jsonname v0.25.5 // indirect
	github.com/go-openapi/swag/jsonutils v0.25.5 // indirect
	github.com/go-openapi/swag/loading v0.25.5 // indirect
	github.com/go-openapi/swag/mangling v0.25.5 // indirect
	github.com/go-openapi/swag/netutils v0.25.5 // indirect
	github.com/go-openapi/swag/stringutils v0.25.5 // indirect
	github.com/go-openapi/swag/typeutils v0.25.5 // indirect
	github.com/go-openapi/swag/yamlutils v0.25.5 // indirect
	github.com/gonvenience/bunt v1.4.3 // indirect
	github.com/gonvenience/idem v0.0.3 // indirect
	github.com/gonvenience/neat v1.3.20 // indirect
	github.com/gonvenience/term v1.0.5 // indirect
	github.com/gonvenience/text v1.0.10 // indirect
	github.com/gonvenience/ytbx v1.5.0 // indirect
	github.com/google/btree v1.1.3 // indirect
	github.com/google/gnostic-models v0.7.1 // indirect
	github.com/gosuri/uitable v0.0.4 // indirect
	github.com/goware/prefixer v0.0.0-20160118172347-395022866408 // indirect
	github.com/hashicorp/logutils v1.0.0 // indirect
	github.com/homeport/dyff v1.12.0 // indirect
	github.com/huaweicloud/huaweicloud-sdk-go-v3 v0.1.195 // indirect
	github.com/ianlancetaylor/demangle v0.0.0-20251118225945-96ee0021ea0f // indirect
	github.com/jmoiron/sqlx v1.4.0 // indirect
	github.com/lann/builder v0.0.0-20180802200727-47ae307949d0 // indirect
	github.com/lann/ps v0.0.0-20150810152359-62de8c46ede0 // indirect
	github.com/lib/pq v1.12.3 // indirect
	github.com/liggitt/tabwriter v0.0.0-20181228230101-89fcab3d43de // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/mattn/go-ciede2000 v0.0.0-20170301095244-782e8c62fec3 // indirect
	github.com/mitchellh/go-ps v1.0.0 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/moby/go-archive v0.2.0 // indirect
	github.com/moby/moby/api v1.54.2 // indirect
	github.com/moby/moby/client v0.4.1 // indirect
	github.com/moby/patternmatcher v0.6.1 // indirect
	github.com/moby/sys/sequential v0.6.0 // indirect
	github.com/moby/sys/user v0.4.0 // indirect
	github.com/moby/sys/userns v0.1.0 // indirect
	github.com/moby/term v0.5.2 // indirect
	github.com/monochromegane/go-gitignore v0.0.0-20200626010858-205db1a8cc00 // indirect
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/peterbourgon/diskv v2.0.1+incompatible // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/rubenv/sql-migrate v1.8.1 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2 // indirect
	github.com/shirou/gopsutil/v4 v4.26.3 // indirect
	github.com/standard-webhooks/standard-webhooks/libraries v0.0.1 // indirect
	github.com/stoewer/go-strcase v1.3.0 // indirect
	github.com/tetratelabs/wabin v0.0.0-20230304001439-f6f874872834 // indirect
	github.com/tetratelabs/wazero v1.12.0 // indirect
	github.com/texttheater/golang-levenshtein v1.0.1 // indirect
	github.com/tjfoc/gmsm v1.4.1 // indirect
	github.com/tklauser/go-sysconf v0.3.16 // indirect
	github.com/tklauser/numcpus v0.11.0 // indirect
	github.com/uber/jaeger-client-go v2.30.0+incompatible // indirect
	github.com/uber/jaeger-lib v2.4.1+incompatible // indirect
	github.com/urfave/cli v1.22.17 // indirect
	github.com/virtuald/go-ordered-json v0.0.0-20170621173500-b18e6e673d74 // indirect
	github.com/xlab/treeprint v1.2.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/proto/otlp v1.10.0 // indirect
	gopkg.in/evanphx/json-patch.v4 v4.13.0 // indirect
	k8s.io/api v0.36.2 // indirect
	k8s.io/apiextensions-apiserver v0.36.1 // indirect
	k8s.io/apiserver v0.36.1 // indirect
	k8s.io/cli-runtime v0.36.2 // indirect
	k8s.io/component-base v0.36.1 // indirect
	k8s.io/kubectl v0.36.1 // indirect
)
