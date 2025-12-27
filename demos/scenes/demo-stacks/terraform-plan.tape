# demo-stacks-terraform-plan scene - Show terraform plan with demo-stacks
Output demo-stacks-terraform-plan.gif
Output demo-stacks-terraform-plan.mp4
Output demo-stacks-terraform-plan.svg

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
Require terraform

# Scene starts
Type "# Plan VPC infrastructure changes"
Enter
Sleep 300ms

# Execute terraform plan
Type "atmos terraform plan vpc -s plat-dev-ue2"
Sleep 2s
Enter
Sleep 5s

# Capture screenshot
Screenshot demo-stacks-terraform-plan.png
Sleep 2s

# Show another component
Type "# Plan database changes"
Enter
Sleep 300ms

Type "atmos terraform plan database -s plat-dev-ue2"
Sleep 2s
Enter
Sleep 4s

# Cleanup
Sleep 1s

# Hold final frame
Sleep 10s
