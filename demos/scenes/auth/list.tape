# list scene - Show configured identities and providers
Output auth-list.gif
Output auth-list.mp4

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
Type "# List configured identities and providers"
Enter
Sleep 1s

Type "# Show identity tree (default view)"
Enter
Sleep 500ms

Type "atmos auth list"
Sleep 500ms
Enter
Sleep 3s

# Capture screenshot
Screenshot auth-list.png

Type "# Table format for quick overview"
Enter
Sleep 500ms

Type "atmos auth list --format table"
Sleep 500ms
Enter
Sleep 3s

Type "# Multiple output formats available"
Enter
Sleep 1s
