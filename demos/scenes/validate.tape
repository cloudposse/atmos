# validate scene - Validate stack configurations
Output validate.gif
Output validate.mp4
Output validate.svg

# VHS Configuration
Source _theme.tape
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
Sleep 2s
Enter
Sleep 3s

# Capture screenshot
Screenshot validate.png

Type "# Catch configuration errors before deployment"
Enter
Sleep 2s

Type "atmos validate component vpc -s plat-dev-ue2"
Sleep 2s
Enter
Sleep 3s

Type "# Ensure infrastructure consistency"
Enter
Sleep 1s


# Hold final frame
Sleep 10s
