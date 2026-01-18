# Demo Component Versions Example
# Pin components to specific versions
Output demo-component-versions.gif
Output demo-component-versions.mp4
Output demo-component-versions.svg

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
Type "# Demo Component Versions: Pin to specific versions"
Enter
Sleep 1s

Type "# View vendor configuration with version refs"
Enter
Sleep 500ms

Type "cat vendor.yaml"
Sleep 1s
Enter
Sleep 3s

# Capture screenshot
Screenshot demo-component-versions.png

Type "# Version pinning ensures reproducible deployments"
Enter
Sleep 2s

# Hold final frame
Sleep 10s
