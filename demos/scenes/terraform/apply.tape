# terraform-apply scene - Deploy with one command
Output terraform-apply.gif
Output terraform-apply.mp4
Output terraform-apply.svg

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
Require terraform

# Scene starts
Type "# Deploy infrastructure with a single command"
Enter
Sleep 1s

Type "atmos terraform apply vpc -s plat-dev-ue2 --auto-approve"
Sleep 2s
Enter
Sleep 4s

# Capture screenshot
Screenshot terraform-apply.png

Sleep 2s

Type "# Deploy with one command - no manual steps"
Enter
Sleep 1s

# Hold final frame
Sleep 10s
