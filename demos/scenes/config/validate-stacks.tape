# validate-stacks scene - Validate all stack configurations
Output config-validate-stacks.gif
Output config-validate-stacks.mp4

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
Type "# Validate all stack configurations"
Enter
Sleep 1s

Type "# Check all stacks against schemas"
Enter
Sleep 500ms

Type "atmos validate stacks"
Sleep 500ms
Enter
Sleep 3s

# Capture screenshot
Screenshot config-validate-stacks.png

Type "# Validate specific stack"
Enter
Sleep 500ms

Type "atmos validate stacks --stack plat-dev-ue2"
Sleep 500ms
Enter
Sleep 3s

Type "# Catch errors before deployment"
Enter
Sleep 1s
