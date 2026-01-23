# Demo Atlantis Example
# Generate Atlantis configuration
Output demo-atlantis.gif
Output demo-atlantis.mp4
Output demo-atlantis.svg

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
Type "# Demo Atlantis: PR-based Terraform automation"
Enter
Sleep 1s

Type "# Generate Atlantis repo configuration"
Enter
Sleep 500ms

Type "atmos atlantis generate repo-config --config-template config-1 --project-template project-1"
Sleep 1s
Enter
Sleep 3s

# Capture screenshot
Screenshot demo-atlantis.png

Type "# Atlantis automates Terraform in PRs"
Enter
Sleep 2s

# Hold final frame
Sleep 10s
