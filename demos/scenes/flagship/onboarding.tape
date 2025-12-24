# onboarding scene - Getting started with Atmos
Output flagship-onboarding.gif
Output flagship-onboarding.mp4
Output flagship-onboarding.svg

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
Type "# Getting started with Atmos"
Enter
Sleep 1s

Type "# Check version and get help"
Enter
Sleep 2s

Type "atmos version"
Sleep 2s
Enter
Sleep 2s

Type "# Explore available commands"
Enter
Sleep 2s

Type "atmos --help"
Sleep 2s
Enter
Sleep 3s

# Capture screenshot
Screenshot flagship-onboarding.png

Type "# Initialize a new project"
Enter
Sleep 2s

Type "atmos --help | head -30"
Sleep 2s
Enter
Sleep 3s

Type "# Start building infrastructure today"
Enter
Sleep 1s

# Hold final frame
Sleep 10s
