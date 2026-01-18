# Demo Env Example
# Export environment variables for providers
Output demo-env.gif
Output demo-env.mp4
Output demo-env.svg

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
Type "# Demo Env: Export environment variables"
Enter
Sleep 1s

Type "# View environment variable exports"
Enter
Sleep 500ms

Type "atmos env"
Sleep 1s
Enter
Sleep 3s

# Capture screenshot
Screenshot demo-env.png

Type "# Use with eval to export to your shell"
Enter
Sleep 2s

# Hold final frame
Sleep 10s
