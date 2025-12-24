# about scene - Show information about atmos
Output about.gif
Output about.mp4
Output about.svg

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
Type "# Learn about atmos"
Enter
Sleep 1s

Type "atmos about"
Sleep 2s
Enter
Sleep 3s

# Capture screenshot
Screenshot about.png

Type "# Quick overview of atmos capabilities"
Enter
Sleep 1s


# Hold final frame
Sleep 10s
