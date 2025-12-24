# destroy scene - Destroy terraform-managed infrastructure
Output terraform-destroy.gif
Output terraform-destroy.mp4
Output terraform-destroy.svg

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
Type "# Destroy infrastructure for a component"
Enter
Sleep 1s

Type "atmos terraform destroy vpc -s plat-dev-ue2"
Sleep 2s
Enter
Sleep 3s

# Capture screenshot
Screenshot terraform-destroy.png

Type "# Safely tear down resources with confirmation"
Enter
Sleep 1s


# Hold final frame
Sleep 10s
