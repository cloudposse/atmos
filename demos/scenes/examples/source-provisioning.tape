# Source Provisioning Example
# Just-in-Time component vendoring
Output source-provisioning.gif
Output source-provisioning.mp4
Output source-provisioning.svg

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
Type "# Source Provisioning: JIT component vendoring"
Enter
Sleep 1s

Type "# Describe source configuration"
Enter
Sleep 500ms

Type "atmos terraform source describe myapp --stack dev"
Sleep 1s
Enter
Sleep 3s

# Capture screenshot
Screenshot source-provisioning.png

Type "# Sources are auto-provisioned when needed"
Enter
Sleep 2s

# Hold final frame
Sleep 10s
