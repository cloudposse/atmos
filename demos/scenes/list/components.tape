# list-components scene - Explore component metadata
Output list-components.gif
Output list-components.mp4

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
Type "# Explore your infrastructure with rich metadata"
Enter
Sleep 1s

Type "atmos list components"
Sleep 500ms
Enter
Sleep 2s

# Capture screenshot
Screenshot list-components.png

Type "# View component configuration with provenance"
Enter
Sleep 500ms

Type "atmos describe component vpc -s plat-dev-ue2 --provenance"
Sleep 500ms
Enter
Sleep 3s

Type "# See where every value comes from"
Enter
Sleep 1s
