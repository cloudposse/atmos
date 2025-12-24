# support scene - Get support information and resources
Output support.gif
Output support.mp4
Output support.svg

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
Type "# Get support and help resources"
Enter
Sleep 1s

Type "atmos support"
Sleep 2s
Enter
Sleep 3s

# Capture screenshot
Screenshot support.png

Type "# Find documentation, community, and support options"
Enter
Sleep 1s


# Hold final frame
Sleep 10s
