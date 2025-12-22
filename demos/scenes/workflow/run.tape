# workflow-run scene - Complex deployments as simple YAML
Output workflow-run.gif
Output workflow-run.mp4

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
Type "# See available workflows"
Enter
Sleep 500ms

Type "cat workflows/deploy.yaml"
Sleep 500ms
Enter
Sleep 3s

# Capture screenshot
Screenshot workflow-run.png

Type "# Run the deployment workflow"
Enter
Sleep 500ms

Type "atmos workflow deploy -f workflows/deploy.yaml"
Sleep 500ms
Enter
Sleep 4s

Type "# Complex deployments become simple YAML"
Enter
Sleep 1s
