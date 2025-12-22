# terraform-plan scene - Basic terraform plan workflow
Output terraform-plan.gif
Output terraform-plan.mp4

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
Type "# Plan infrastructure changes before deploying"
Enter
Sleep 1s

Type "atmos terraform plan vpc -s plat-dev-ue2"
Sleep 500ms
Enter
Sleep 3s

# Capture screenshot at plan output
Screenshot terraform-plan.png

Sleep 2s

Type "# See exactly what will change before deploying"
Enter
Sleep 1s
