# docs scene - Access documentation from CLI
Output docs.gif
Output docs.mp4
Output docs.svg

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
Type "# Access documentation directly from the CLI"
Enter
Sleep 1s

Type "atmos docs"
Sleep 2s
Enter
Sleep 3s

# Capture screenshot
Screenshot docs.png

Type "# Opens atmos.tools in your browser"
Enter
Sleep 2s

Type "# View component documentation"
Enter
Sleep 300ms

Type "atmos docs vpc"
Sleep 2s
Enter
Sleep 3s

Type "# Quick access to component README files"
Enter
Sleep 1s


# Hold final frame
Sleep 10s
