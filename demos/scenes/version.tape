# version scene - Check atmos version and updates
Output version.gif
Output version.mp4
Output version.svg

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
Type "# Check atmos version and available updates"
Enter
Sleep 1s

Type "atmos version"
Sleep 2s
Enter
Sleep 3s

# Capture screenshot
Screenshot version.png

Type "# List available versions"
Enter
Sleep 2s

Type "atmos version list"
Sleep 2s
Enter
Sleep 3s

Type "# Stay up-to-date with new features"
Enter
Sleep 1s


# Hold final frame
Sleep 10s
