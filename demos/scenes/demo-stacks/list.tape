# demo-stacks-list scene - Show list commands
Output demo-stacks-list.gif

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

Type "# List all available stacks"
Enter
Sleep 300ms

# List stacks
Type "atmos list stacks"
Sleep 500ms
Enter
Sleep 2s

# Capture screenshot
Screenshot demo-stacks-list-stacks.png
Sleep 1s

Type "# List all components in a stack"
Enter
Sleep 300ms

Type "atmos list components --stack dev"
Sleep 500ms
Enter
Sleep 2s

Screenshot demo-stacks-list-components.png
Sleep 1s

# Cleanup
Sleep 1s
