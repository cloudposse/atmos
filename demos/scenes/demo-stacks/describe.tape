# demo-stacks-describe scene - Show list and describe commands
Output demo-stacks-describe.gif
Output demo-stacks-describe.mp4
Output demo-stacks-describe.svg

# VHS Configuration
Source ../../scenes/_theme.tape
Set Width 1400
Set Height 900

# Fix prompt (VHS runs with --no-rcs)
Hide
Type "PS1='\033[38;2;90;86;224m> \033[0m'"
Enter
Ctrl+L
Show

# Dependencies
Require atmos

# Scene starts
Type "# List all available stacks"
Enter
Sleep 300ms

# Execute list stacks
Type "atmos list stacks"
Sleep 2s
Enter
Sleep 3s

# Capture screenshot
Screenshot demo-stacks-describe.png
Sleep 2s

# Show component with provenance
Type "# View VPC configuration with provenance"
Enter
Sleep 300ms

Type "atmos describe component vpc -s plat-dev-ue2 --provenance"
Sleep 2s
Enter
Sleep 3s

# Cleanup
Sleep 1s

# Hold final frame
Sleep 10s
