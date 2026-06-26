# Quick Start Advanced Example
# Complete multi-environment setup
Output quick-start-advanced.gif
Output quick-start-advanced.mp4
Output quick-start-advanced.svg

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
Type "# Quick Start Advanced: Multi-environment setup"
Enter
Sleep 1s

Type "# Validate all stacks"
Enter
Sleep 500ms

Type "atmos validate stacks"
Sleep 1s
Enter
Sleep 2s

Type "# Describe all stacks"
Enter
Sleep 500ms

Type "atmos describe stacks"
Sleep 1s
Enter
Sleep 3s

# Capture screenshot
Screenshot quick-start-advanced.png

Type "# Advanced patterns for real-world infrastructure"
Enter
Sleep 2s

# Hold final frame
Sleep 10s
