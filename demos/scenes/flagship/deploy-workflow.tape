# deploy-workflow scene - Full deployment workflow
Output flagship-deploy-workflow.gif
Output flagship-deploy-workflow.mp4

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
Type "# Complete deployment workflow with Atmos"
Enter
Sleep 1s

Type "# First, see what we're working with"
Enter
Sleep 500ms

Type "atmos list stacks"
Sleep 500ms
Enter
Sleep 2s

Type "# Validate before deploying"
Enter
Sleep 500ms

Type "atmos validate stacks --stack plat-dev-ue2"
Sleep 500ms
Enter
Sleep 2s

# Capture screenshot
Screenshot flagship-deploy-workflow.png

Type "# Deploy using a workflow"
Enter
Sleep 500ms

Type "atmos workflow deploy -f deploy --stack plat-dev-ue2"
Sleep 500ms
Enter
Sleep 3s

Type "# Infrastructure as code at scale"
Enter
Sleep 1s
