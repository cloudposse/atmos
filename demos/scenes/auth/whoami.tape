# whoami scene - Show current authenticated identity
Output auth-whoami.gif
Output auth-whoami.mp4
Output auth-whoami.svg

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
Type "# Show your current authenticated identity"
Enter
Sleep 1s

Type "atmos auth whoami"
Sleep 2s
Enter
Sleep 3s

# Capture screenshot
Screenshot auth-whoami.png

Type "# Quick identity check across all providers"
Enter
Sleep 1s


# Hold final frame
Sleep 10s
