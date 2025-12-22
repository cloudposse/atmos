# vendor-pull scene - Pin and pull component dependencies
Output vendor-pull.gif
Output vendor-pull.mp4

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
Type "# Pull components from remote sources"
Enter
Sleep 1s

Type "# View vendor configuration"
Enter
Sleep 500ms

Type "cat vendor.yaml"
Sleep 500ms
Enter
Sleep 2s

# Capture screenshot
Screenshot vendor-pull.png

Type "# Pull vendored components"
Enter
Sleep 500ms

Type "atmos vendor pull"
Sleep 500ms
Enter
Sleep 3s

Type "# Pin versions, pull dependencies - reproducible builds"
Enter
Sleep 1s
