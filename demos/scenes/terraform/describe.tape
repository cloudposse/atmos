# terraform-describe scene - Describe component configuration
Output terraform-describe.gif
Output terraform-describe.mp4

# VHS Configuration
Source ../../scenes/_theme.tape
Set Width 1400
Set Height 800

# Fix prompt (VHS runs with --no-rcs)
Hide
Type "PS1='\033[38;2;90;86;224m> \033[0m'"
Enter
Ctrl+L
Show

# Dependencies
Require atmos

# Scene starts
Type "# See component configuration with provenance"
Enter
Sleep 1s

Type "atmos describe component vpc -s plat-dev-ue2 --provenance"
Sleep 500ms
Enter
Sleep 3s

# Capture screenshot
Screenshot terraform-describe.png

Type "# Compare across environments"
Enter
Sleep 500ms

Type "atmos describe component vpc -s plat-prod-ue2 --provenance"
Sleep 500ms
Enter
Sleep 3s

Type "# See exactly where each value comes from"
Enter
Sleep 1s
