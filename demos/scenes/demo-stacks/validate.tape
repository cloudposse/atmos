# demo-stacks-validate scene - Show validate stacks command
Output demo-stacks-validate.gif

# VHS Configuration
Set Theme "Catppuccin Mocha"
Set Width 1400
Set Height 800
Set FontSize 14
Set TypingSpeed 50ms

# Dependencies
Require atmos

# Setup
Type "cd examples/demo-stacks"
Enter
Sleep 500ms

Type "# Validate stack configurations"
Enter
Sleep 300ms

# Execute validate stacks
Type "atmos validate stacks"
Sleep 500ms
Enter
Sleep 3s

# Capture screenshot
Screenshot demo-stacks-validate.png
Sleep 1s

# Show component validation
Type "# Validate specific component"
Enter
Sleep 300ms

Type "atmos validate component myapp --stack dev"
Sleep 500ms
Enter
Sleep 2s

# Cleanup
Sleep 1s
