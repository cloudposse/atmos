# Demo Helmfile Example
# Deploy Kubernetes resources with Helmfile
Output demo-helmfile.gif
Output demo-helmfile.mp4
Output demo-helmfile.svg

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
Type "# Demo Helmfile: Kubernetes deployments with Atmos"
Enter
Sleep 1s

Type "# View the helmfile component configuration"
Enter
Sleep 500ms

Type "atmos describe component demo -s dev"
Sleep 1s
Enter
Sleep 3s

# Capture screenshot
Screenshot demo-helmfile.png

Type "# Helmfile integrates with stack patterns"
Enter
Sleep 2s

# Hold final frame
Sleep 10s
