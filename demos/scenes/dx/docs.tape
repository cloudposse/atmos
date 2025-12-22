# docs scene - Access documentation from CLI
Output dx-docs.gif
Output dx-docs.mp4

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
Type "# Access documentation directly from the CLI"
Enter
Sleep 1s

Type "atmos docs"
Sleep 500ms
Enter
Sleep 3s

# Capture screenshot
Screenshot dx-docs.png

Type "# Opens atmos.tools in your browser"
Enter
Sleep 500ms

Type "atmos docs terraform"
Sleep 500ms
Enter
Sleep 3s

Type "# Quick access to any command's docs"
Enter
Sleep 1s
