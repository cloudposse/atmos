# theme scene - Configure terminal theme and colors
Output theme.gif
Output theme.mp4
Output theme.svg

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
Type "# Configure terminal theme and colors"
Enter
Sleep 1s

Type "# List all available themes"
Enter
Sleep 300ms

Type "atmos theme list"
Sleep 2s
Enter
Sleep 3s

# Capture screenshot
Screenshot theme.png

Type "# Show the current active theme"
Enter
Sleep 300ms

Type "atmos theme show"
Sleep 2s
Enter
Sleep 3s

Type "# 300+ themes to choose from"
Enter
Sleep 1s

# Hold final frame
Sleep 10s
