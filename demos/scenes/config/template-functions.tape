# template-functions scene - Use template functions in stacks
Output config-template-functions.gif
Output config-template-functions.mp4
Output config-template-functions.svg

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
Type "# Use template functions for dynamic configuration"
Enter
Sleep 1s

Type "# View stack with template functions"
Enter
Sleep 2s

Type "cat stacks/catalog/api.yaml"
Sleep 2s
Enter
Sleep 3s

# Capture screenshot
Screenshot config-template-functions.png

Type "# Resolved configuration with values from other components"
Enter
Sleep 2s

Type "atmos describe component api -s plat-dev-ue2"
Sleep 2s
Enter
Sleep 3s

Type "# Dynamic values reduce duplication"
Enter
Sleep 1s

# Hold final frame
Sleep 10s
