# Demo Stacks Example
# Stack inheritance across environments
Output demo-stacks.gif
Output demo-stacks.mp4
Output demo-stacks.svg

# VHS Configuration
Source "../_theme.tape"
Set Width 1400
Set Height 800

# Enable TrueColor for proper color rendering
Env COLORTERM "truecolor"

# Fix prompt (VHS runs with --no-rcs)
Env PS1 "\033[38;2;90;86;224m> \033[0m"

Hide
Enter
Ctrl+L
Show

# Dependencies
Require atmos

# Scene starts
Type "# Demo Stacks: Inheritance across environments"
Enter
Sleep 1s

Type "# See configuration inheritance in dev"
Enter
Sleep 500ms

Type "atmos describe component myapp -s dev"
Sleep 1s
Enter
Sleep 3s

Type "# Compare with production"
Enter
Sleep 500ms

Type "atmos describe component myapp -s prod"
Sleep 1s
Enter
Sleep 3s

# Capture screenshot
Screenshot demo-stacks.png

Type "# See stacks overview"
Enter
Sleep 500ms

Type "atmos describe stacks --components myapp"
Sleep 1s
Enter
Sleep 3s

# Hold final frame
Sleep 10s
