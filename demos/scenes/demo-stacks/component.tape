# demo-stacks-component scene - Show describe component command
Output demo-stacks-component.gif
Output demo-stacks-component.mp4
Output demo-stacks-component.svg

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
Type "# Describe database component in dev"
Enter
Sleep 300ms

# Execute describe component
Type "atmos describe component database -s plat-dev-ue2"
Sleep 2s
Enter
Sleep 3s

# Capture screenshot showing component configuration
Screenshot demo-stacks-component.png
Sleep 2s

# Show component in different stack
Type "# Compare with production configuration"
Enter
Sleep 300ms

Type "atmos describe component database -s plat-prod-ue2"
Sleep 2s
Enter
Sleep 3s

# Cleanup
Sleep 1s

# Hold final frame
Sleep 10s
