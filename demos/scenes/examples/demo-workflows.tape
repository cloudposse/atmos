# Demo Workflows Example
# Run multiple Atmos commands in sequence
Output demo-workflows.gif
Output demo-workflows.mp4
Output demo-workflows.svg

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
Type "# Demo Workflows: Run multiple commands in sequence"
Enter
Sleep 1s

Type "# List available workflows"
Enter
Sleep 500ms

Type "atmos describe workflows"
Sleep 1s
Enter
Sleep 3s

# Capture screenshot
Screenshot demo-workflows.png

Type "# Workflows define multi-step operations"
Enter
Sleep 2s

# Hold final frame
Sleep 10s
