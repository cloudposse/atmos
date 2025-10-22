package cobra

// Command is a mock cobra.Command for testing.
type Command struct {
	Use     string
	Short   string
	Long    string
	Example string
	Args    func()
	RunE    func(cmd *Command, args []string) error
}
