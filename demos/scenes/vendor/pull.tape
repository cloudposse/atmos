# vendor-pull scene - Pin and pull component dependencies
Output vendor-pull.gif
Output vendor-pull.mp4
Output vendor-pull.svg

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
Type "# List vendored components"
Enter
Sleep 2s

Type "atmos list vendor"
Sleep 2s
Enter
Sleep 2s

# Capture screenshot
Screenshot vendor-pull.png

Type "# Pull all vendored components"
Enter
Sleep 2s

Type "atmos vendor pull"
Sleep 2s
Enter
Sleep 8s

Type "# Pull only networking components by tag"
Enter
Sleep 2s

Type "atmos vendor pull --tags networking"
Sleep 2s
Enter
Sleep 6s

Type "# Pin versions, pull by tags - reproducible builds"
Enter
Sleep 1s

# Hold final frame
Sleep 10s
