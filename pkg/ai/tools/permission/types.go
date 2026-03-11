package permission

// Mode represents the permission checking mode.
type Mode string

const (
	// ModePrompt always prompts the user for permission.
	ModePrompt Mode = "prompt"
	// ModeAllow automatically allows all tools.
	ModeAllow Mode = "allow"
	// ModeDeny automatically denies all tools.
	ModeDeny Mode = "deny"
	// ModeYOLO bypasses all permission checks (dangerous).
	ModeYOLO Mode = "yolo"
)

// Config holds permission configuration.
type Config struct {
	Mode            Mode     `yaml:"mode" json:"mode" mapstructure:"mode"`
	AllowedTools    []string `yaml:"allowed_tools" json:"allowed_tools" mapstructure:"allowed_tools"`
	RestrictedTools []string `yaml:"restricted_tools" json:"restricted_tools" mapstructure:"restricted_tools"`
	BlockedTools    []string `yaml:"blocked_tools" json:"blocked_tools" mapstructure:"blocked_tools"`
	YOLOMode        bool     `yaml:"yolo_mode" json:"yolo_mode" mapstructure:"yolo_mode"`
	AuditEnabled    bool     `yaml:"audit_enabled" json:"audit_enabled" mapstructure:"audit_enabled"`
	AuditPath       string   `yaml:"audit_path" json:"audit_path" mapstructure:"audit_path"`
}

// Decision represents a permission decision.
type Decision struct {
	Allowed bool
	Reason  string
}

// AuditEntry represents a single audit log entry.
type AuditEntry struct {
	Timestamp  string                 `json:"timestamp"`
	User       string                 `json:"user,omitempty"`
	SessionID  string                 `json:"session_id,omitempty"`
	Tool       string                 `json:"tool"`
	Params     map[string]interface{} `json:"params"`
	Permission string                 `json:"permission"` // allowed, denied, blocked
	Result     string                 `json:"result"`     // success, failure
}
