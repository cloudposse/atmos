# validate scene - Validate stack configurations
Output dx-validate.gif
Output dx-validate.mp4

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
Type "# Validate your stack configurations"
Enter
Sleep 1s

Type "atmos validate stacks"
Sleep 500ms
Enter
Sleep 3s

# Capture screenshot
Screenshot dx-validate.png

Type "# Catch configuration errors before deployment"
Enter
Sleep 500ms

Type "atmos validate component vpc -s plat-dev-ue2"
Sleep 500ms
Enter
Sleep 3s

Type "# Ensure infrastructure consistency"
Enter
Sleep 1s
