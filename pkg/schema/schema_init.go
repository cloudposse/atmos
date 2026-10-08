package schema

// DefaultInitRepository hosts the official examples used by atmos init.
const DefaultInitRepository = "github.com/cloudposse/atmos"

// InitConfig configures source resolution for atmos init.
type InitConfig struct {
	// Repository resolves relative source paths after named templates.
	Repository string `yaml:"repository,omitempty" json:"repository,omitempty" mapstructure:"repository"`
	// Ref selects the source revision when neither the source URL nor --ref pins one.
	Ref string `yaml:"ref,omitempty" json:"ref,omitempty" mapstructure:"ref"`
	// Git controls creation of a Git repository and initial commit, defaulting to true.
	Git *bool `yaml:"git,omitempty" json:"git,omitempty" mapstructure:"git"`
}
