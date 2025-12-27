# console scene - Open cloud provider console in browser
Output auth-console.gif
Output auth-console.mp4
Output auth-console.svg

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
Type "# Open cloud provider console in browser"
Enter
Sleep 1s

Type "# Launch AWS console with current credentials"
Enter
Sleep 2s

Type "atmos auth console"
Sleep 2s
Enter
Sleep 3s

# Capture screenshot
Screenshot auth-console.png

Type "# Open specific service console"
Enter
Sleep 2s

Type "atmos auth console --destination s3"
Sleep 2s
Enter
Sleep 3s

Type "# Browser opens automatically"
Enter
Sleep 1s

# Hold final frame
Sleep 10s
