# demo-stacks-describe scene - Show describe stacks command
Output demo-stacks-describe.gif

# VHS Configuration
Set Theme "Catppuccin Mocha"
Set Width 1400
Set Height 900
Set FontSize 14
Set TypingSpeed 50ms

# Dependencies
Require atmos

# Setup
Type "# Navigate to demo-stacks example"
Enter
Sleep 300ms

Type "cd examples/demo-stacks"
Enter
Sleep 500ms

Type "# Describe all stacks to see configuration"
Enter
Sleep 300ms

# Execute describe stacks
Type "atmos describe stacks"
Sleep 500ms
Enter
Sleep 3s

# Capture screenshot
Screenshot demo-stacks-describe.png
Sleep 2s

# Show specific stack
Type "# View specific stack: dev"
Enter
Sleep 300ms

Type "atmos describe stacks --stack dev"
Sleep 500ms
Enter
Sleep 2s

# Cleanup
Sleep 1s
