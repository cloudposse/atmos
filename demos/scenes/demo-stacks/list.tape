# demo-stacks-list scene - Show list commands
Output demo-stacks-list.gif
Output demo-stacks-list.mp4
Output demo-stacks-list.svg

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
Type "# List all available stacks"
Enter
Sleep 300ms

# List stacks
Type "atmos list stacks"
Sleep 2s
Enter
Sleep 2s

# Capture screenshot
Screenshot demo-stacks-list.png
Sleep 1s

Type "# List all components across stacks"
Enter
Sleep 300ms

Type "atmos list components"
Sleep 2s
Enter
Sleep 3s

# Cleanup
Sleep 1s

# Hold final frame
Sleep 10s
