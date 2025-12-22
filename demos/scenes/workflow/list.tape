# workflow-list scene - Discover available workflows
Output workflow-list.gif
Output workflow-list.mp4

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
Type "# See all available workflows"
Enter
Sleep 1s

Type "ls workflows/"
Sleep 500ms
Enter
Sleep 2s

Type "# View workflow definitions"
Enter
Sleep 500ms

Type "cat workflows/validate.yaml"
Sleep 500ms
Enter
Sleep 3s

# Capture screenshot
Screenshot workflow-list.png

Type "# Self-documenting automation"
Enter
Sleep 1s
