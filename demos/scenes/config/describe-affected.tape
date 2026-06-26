# describe-affected scene - Find affected components from changes
Output config-describe-affected.gif
Output config-describe-affected.mp4
Output config-describe-affected.svg

# VHS Configuration
Source "../../scenes/_theme.tape"
Set Width 1400
Set Height 800

# Enable TrueColor for proper color rendering in VHS
Env COLORTERM "truecolor"

# Fix prompt (VHS runs with --no-rcs)
Env PS1 "\033[38;2;90;86;224m> \033[0m"

Hide
Enter
Ctrl+L
Show

# Dependencies
Require atmos

# Scene starts
Type "# Find components affected by changes"
Enter
Sleep 1s

Type "# Filter to a single component using --query"
Enter
Sleep 2s

Type `atmos describe affected --ref main --query '[.[] | select(.component == "vpc")]'`
Sleep 2s
Enter
Sleep 3s

# Capture screenshot
Screenshot config-describe-affected.png

Type "# Output as JSON for CI/CD pipelines"
Enter
Sleep 2s

Type `atmos describe affected --ref main --format json --query '[.[] | select(.component == "vpc")] | .[0]'`
Sleep 2s
Enter
Sleep 3s

Type "# Query filtering reduces output to what you need"
Enter
Sleep 1s

# Hold final frame
Sleep 10s
