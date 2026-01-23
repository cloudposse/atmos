# Quick Start Simple Example
# Minimal Atmos setup with a single component
Output quick-start-simple.gif
Output quick-start-simple.mp4
Output quick-start-simple.svg

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
Type "# Quick Start: Minimal Atmos setup"
Enter
Sleep 1s

Type "# List all stacks"
Enter
Sleep 500ms

Type "atmos list stacks"
Sleep 1s
Enter
Sleep 2s

Type "# Describe the station component"
Enter
Sleep 500ms

Type "atmos describe component station -s dev"
Sleep 1s
Enter
Sleep 3s

# Capture screenshot
Screenshot quick-start-simple.png

Type "# Ready for terraform operations"
Enter
Sleep 2s

# Hold final frame
Sleep 10s
