# describe-affected scene - Find affected components from changes
Output config-describe-affected.gif
Output config-describe-affected.mp4

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
Type "# Find components affected by changes"
Enter
Sleep 1s

Type "# Compare HEAD against main branch"
Enter
Sleep 500ms

Type "atmos describe affected --ref main"
Sleep 500ms
Enter
Sleep 3s

# Capture screenshot
Screenshot config-describe-affected.png

Type "# Get affected stacks for CI/CD pipelines"
Enter
Sleep 500ms

Type "atmos describe affected --ref main --format json"
Sleep 500ms
Enter
Sleep 3s

Type "# Deploy only what changed"
Enter
Sleep 1s
