# Demo Vendoring Example
# Pull Terraform modules from external sources
Output demo-vendoring.gif
Output demo-vendoring.mp4
Output demo-vendoring.svg

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
Type "# Demo Vendoring: Pull modules from external sources"
Enter
Sleep 1s

Type "# View the vendor configuration"
Enter
Sleep 500ms

Type "cat vendor.yaml"
Sleep 1s
Enter
Sleep 3s

Type "# Pull all vendored components"
Enter
Sleep 500ms

Type "atmos vendor pull"
Sleep 1s
Enter
Sleep 4s

# Capture screenshot
Screenshot demo-vendoring.png

Type "# Vendored components are now available"
Enter
Sleep 2s

# Hold final frame
Sleep 10s
