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
Type "# Configure terminal theme"
Enter
Sleep 1s

Type "atmos theme"
Sleep 2s
Enter
Sleep 3s

# Capture screenshot
Screenshot theme.png

Type "# Customize colors and visual appearance"
Enter
Sleep 1s


# Hold final frame
Sleep 10s
