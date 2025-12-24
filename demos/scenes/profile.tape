# profile scene - Manage atmos profiles
Output profile.gif
Output profile.mp4
Output profile.svg

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
Type "# View and manage atmos profiles"
Enter
Sleep 1s

Type "atmos profile"
Sleep 2s
Enter
Sleep 3s

# Capture screenshot
Screenshot profile.png

Type "# Configure different profiles for various environments"
Enter
Sleep 1s


# Hold final frame
Sleep 10s
