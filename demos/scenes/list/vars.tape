# list-vars scene - List component vars across stacks
Output list-vars.gif
Output list-vars.mp4

# VHS Configuration
Source ../../scenes/_theme.tape
Set Width 1400
Set Height 800

# Fix prompt (VHS runs zsh with --no-rcs, use \033 for colors)
Hide
Type "PS1='\033[38;2;90;86;224m> \033[0m'"
Enter
Ctrl+L
Show

# Dependencies
Require atmos

# Scene starts
Type "# List component vars across all stacks"
Enter
Sleep 500ms

Type "atmos list vars vpc"
Sleep 500ms
Enter
Sleep 3s

# Capture screenshot
Screenshot list-vars.png

Type "# Compare vars across environments"
Enter
Sleep 300ms

Type "atmos list vars vpc --query .vars.vpc_cidr"
Sleep 500ms
Enter
Sleep 3s

Type "# See all variable values at a glance"
Enter
Sleep 1s
