# demo-stacks-terraform-plan scene - Show terraform plan with demo-stacks
Output demo-stacks-terraform-plan.gif

# VHS Configuration
Set Theme "Catppuccin Mocha"
Set Width 1400
Set Height 900
Set FontSize 14
Set TypingSpeed 50ms

# Dependencies
Require atmos
Require terraform

# Setup
Type "cd examples/demo-stacks"
Enter
Sleep 500ms

Type "# Initialize and plan terraform component"
Enter
Sleep 300ms

# First show what we're planning
Type "atmos describe component myapp --stack dev | grep -A 5 vars"
Sleep 500ms
Enter
Sleep 2s

Type "# Run terraform plan for myapp in dev stack"
Enter
Sleep 300ms

# Execute terraform plan
Type "atmos terraform plan myapp --stack dev"
Sleep 500ms
Enter
Sleep 5s

# Capture screenshot
Screenshot demo-stacks-terraform-plan.png
Sleep 2s

# Cleanup
Sleep 1s
