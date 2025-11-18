# demo-stacks-component scene - Show describe component command
Output demo-stacks-component.gif

# VHS Configuration
Set Theme "Catppuccin Mocha"
Set Width 1400
Set Height 900
Set FontSize 14
Set TypingSpeed 50ms

# Dependencies
Require atmos

# Setup
Type "cd examples/demo-stacks"
Enter
Sleep 500ms

Type "# Describe a component in a specific stack"
Enter
Sleep 300ms

# Execute describe component
Type "atmos describe component myapp --stack dev"
Sleep 500ms
Enter
Sleep 3s

# Capture screenshot showing component configuration
Screenshot demo-stacks-component.png
Sleep 2s

# Show component in different stack
Type "# View same component in staging"
Enter
Sleep 300ms

Type "atmos describe component myapp --stack staging"
Sleep 500ms
Enter
Sleep 2s

# Show component in prod
Type "# View same component in prod"
Enter
Sleep 300ms

Type "atmos describe component myapp --stack prod"
Sleep 500ms
Enter
Sleep 2s

# Cleanup
Sleep 1s
