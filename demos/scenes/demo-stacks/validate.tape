# demo-stacks-validate scene - Show validate stacks command
Output demo-stacks-validate.gif
Output demo-stacks-validate.mp4

# VHS Configuration
Source ../../scenes/_theme.tape
Set Width 1400
Set Height 800

# Fix prompt (VHS runs with --no-rcs)
Hide
Type "PS1='\033[38;2;90;86;224m> \033[0m'"
Enter
Ctrl+L
Show

# Dependencies
Require atmos

# Scene starts
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

Type "atmos validate component vpc -s plat-dev-ue2"
Sleep 500ms
Enter
Sleep 2s

# Cleanup
Sleep 1s
