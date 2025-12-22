# terraform-auth scene - Use authenticated Terraform commands
Output auth-terraform.gif
Output auth-terraform.mp4

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
Type "# Run terraform with team credentials"
Enter
Sleep 1s

Type "# Credentials managed by Atmos Pro"
Enter
Sleep 500ms

Type "atmos terraform plan vpc -s plat-dev-ue2"
Sleep 500ms
Enter
Sleep 3s

# Capture screenshot
Screenshot auth-terraform.png

Type "# Apply changes with team audit trail"
Enter
Sleep 500ms

Type "atmos terraform apply vpc -s plat-dev-ue2"
Sleep 500ms
Enter
Sleep 3s

Type "# Secure, audited infrastructure management"
Enter
Sleep 1s
