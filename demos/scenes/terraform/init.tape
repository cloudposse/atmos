# init scene - Initialize terraform for a component
Output terraform-init.gif
Output terraform-init.mp4
Output terraform-init.svg

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
Type "# Initialize terraform for a component"
Enter
Sleep 1s

Type "atmos terraform init vpc -s plat-dev-ue2"
Sleep 2s
Enter
Sleep 3s

# Capture screenshot
Screenshot terraform-init.png

Type "# Downloads providers and modules for the component"
Enter
Sleep 1s


# Hold final frame
Sleep 10s
