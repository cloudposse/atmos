# list-stacks scene - List and explore stacks
Output list-stacks.gif
Output list-stacks.mp4

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
Type "# Explore your infrastructure at a glance"
Enter
Sleep 1s

Type "atmos list stacks"
Sleep 500ms
Enter
Sleep 2s

# Capture screenshot
Screenshot list-stacks.png

Type "# List components across all stacks"
Enter
Sleep 500ms

Type "atmos list components"
Sleep 500ms
Enter
Sleep 2s

Type "# Explore, compare, understand your infrastructure"
Enter
Sleep 1s
