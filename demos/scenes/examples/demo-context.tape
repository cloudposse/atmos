# Demo Context Example
# Inspect how Atmos resolves configuration
Output demo-context.gif
Output demo-context.mp4
Output demo-context.svg

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
Type "# Demo Context: Configuration resolution"
Enter
Sleep 1s

Type "# See the resolved Atmos configuration"
Enter
Sleep 500ms

Type "atmos describe config"
Sleep 1s
Enter
Sleep 3s

Type "# Inspect component configuration"
Enter
Sleep 500ms

Type "atmos describe component demo -s acme-west-dev"
Sleep 1s
Enter
Sleep 3s

# Capture screenshot
Screenshot demo-context.png

Type "# Context providers add dynamic values"
Enter
Sleep 2s

# Hold final frame
Sleep 10s
