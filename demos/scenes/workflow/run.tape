# workflow-run scene - Complex deployments as simple YAML
Output workflow-run.gif
Output workflow-run.mp4
Output workflow-run.svg

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
Type "# Run a workflow by name - Atmos finds it automatically"
Enter
Sleep 2s

Type "atmos workflow deploy"
Sleep 2s
Enter
Sleep 4s

# Capture screenshot
Screenshot workflow-run.png

Type "# Or explicitly select the workflow file"
Enter
Sleep 2s

Type "atmos workflow deploy -f networking"
Sleep 2s
Enter
Sleep 4s

Type "# Complex deployments become simple commands"
Enter
Sleep 1s

# Hold final frame
Sleep 10s
