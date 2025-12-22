# terraform-affected scene - Smart deployments with affected detection
Output terraform-affected.gif
Output terraform-affected.mp4

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
Require terraform

# Scene starts
Type "# Only deploy components that changed"
Enter
Sleep 1s

Type "# Check what's affected by recent changes"
Enter
Sleep 500ms

Type "atmos describe affected"
Sleep 500ms
Enter
Sleep 3s

# Capture screenshot
Screenshot terraform-affected.png

Type "# Include dependent components too"
Enter
Sleep 500ms

Type "atmos describe affected --include-dependents"
Sleep 500ms
Enter
Sleep 3s

Type "# Smart deployments - only what changed, plus dependents"
Enter
Sleep 1s
