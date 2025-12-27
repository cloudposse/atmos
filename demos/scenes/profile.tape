# profile scene - Manage atmos profiles
Output profile.gif
Output profile.mp4
Output profile.svg

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
Type "# View and manage atmos profiles"
Enter
Sleep 1s

Type "# List all configured profiles"
Enter
Sleep 300ms

Type "atmos profile list"
Sleep 2s
Enter
Sleep 3s

# Capture screenshot
Screenshot profile.png

Type "# Show details of a specific profile"
Enter
Sleep 300ms

Type "atmos profile show developer"
Sleep 2s
Enter
Sleep 3s

Type "# Use profiles with the --profile flag"
Enter
Sleep 300ms

Type "atmos list stacks --profile ci"
Sleep 2s
Enter
Sleep 3s

Type "# Or set ATMOS_PROFILE environment variable"
Enter
Sleep 300ms

Type "export ATMOS_PROFILE=production"
Sleep 1s
Enter
Sleep 1s

Type "atmos list stacks"
Sleep 2s
Enter
Sleep 3s

Type "# Profiles provide environment-specific config overrides"
Enter
Sleep 1s

# Hold final frame
Sleep 10s
