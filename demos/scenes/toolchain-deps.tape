# Stack Dependencies scene - Tools auto-installed per component
Output toolchain-deps.gif
Output toolchain-deps.mp4
Output toolchain-deps.svg

# VHS Configuration
Source _theme.tape
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
Type "# Define tool dependencies per component"
Enter
Sleep 1s

Type "# Stack config specifies OpenTofu version"
Enter
Sleep 300ms

Type "cat stacks/orgs/acme/plat/dev/us-east-2/dev-ue2.yaml | head -25"
Sleep 1s
Enter
Sleep 3s

# Capture screenshot
Screenshot toolchain-deps.png

Type "# Running terraform auto-installs the required tool"
Enter
Sleep 300ms

Type "atmos terraform plan vpc -s acme-plat-dev-ue2"
Sleep 2s
Enter
Sleep 5s

Type "# Different stacks can use different tool versions"
Enter
Sleep 300ms

Type "# Global, component-type, or component-specific scopes"
Enter
Sleep 1s

Type "# Tool constraints use semver: ~> 1.8.0, ^1.0.0, >= 1.5"
Enter
Sleep 1s

# Hold final frame
Sleep 10s
