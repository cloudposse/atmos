# workflow-list scene - Discover available workflows
Output workflow-list.gif
Output workflow-list.mp4
Output workflow-list.svg

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
Type "# List all available workflows"
Enter
Sleep 2s

Type "atmos list workflows"
Sleep 2s
Enter
Sleep 2s

# Capture screenshot
Screenshot workflow-list.png

Type "# View a workflow definition"
Enter
Sleep 2s

Type "cat workflows/deploy.yaml"
Sleep 2s
Enter
Sleep 3s

Type "# Self-documenting automation"
Enter
Sleep 1s

# Hold final frame
Sleep 10s
