# terraform-output scene - View terraform outputs in various formats
Output terraform-output.gif
Output terraform-output.mp4
Output terraform-output.svg

# VHS Configuration
Source ../../scenes/_theme.tape
Set Width 1400
Set Height 800

# Enable TrueColor for proper color rendering in VHS
Env COLORTERM "truecolor"

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
Type "# View terraform outputs in a beautiful table"
Enter
Sleep 1s

Type "atmos terraform output vpc -s plat-dev-ue2 --format table"
Sleep 2s
Enter
Sleep 3s

# Capture screenshot at table output
Screenshot terraform-output.png

Type "# Export as environment variables for CI/CD"
Enter
Sleep 1s

Type "atmos terraform output vpc -s plat-dev-ue2 --format env"
Sleep 2s
Enter
Sleep 3s

Type "# Or as JSON for automation"
Enter
Sleep 1s

Type "atmos terraform output vpc -s plat-dev-ue2 --format json"
Sleep 2s
Enter
Sleep 3s

Type "# Multiple formats for any workflow"
Enter
Sleep 1s

# Hold final frame
Sleep 10s
