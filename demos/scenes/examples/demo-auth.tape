# Demo Auth Example
# Authenticate with cloud providers
Output demo-auth.gif
Output demo-auth.mp4
Output demo-auth.svg

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
Type "# Demo Auth: Cloud provider authentication"
Enter
Sleep 1s

Type "# View auth configuration"
Enter
Sleep 500ms

Type "atmos describe config | grep -A20 auth"
Sleep 1s
Enter
Sleep 3s

# Capture screenshot
Screenshot demo-auth.png

Type "# Auth integrates with your cloud providers"
Enter
Sleep 2s

# Hold final frame
Sleep 10s
