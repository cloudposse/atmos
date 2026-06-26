# Config Profiles Example
# Manage environment-specific configurations
Output config-profiles.gif
Output config-profiles.mp4
Output config-profiles.svg

# VHS Configuration
Source "../_theme.tape"
Set Width 1400
Set Height 800

# Enable TrueColor for proper color rendering
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
Type "# Config Profiles: Environment-specific configurations"
Enter
Sleep 1s

Type "# List available profiles"
Enter
Sleep 500ms

Type "atmos profile list"
Sleep 1s
Enter
Sleep 3s

# Capture screenshot
Screenshot config-profiles.png

Type "# Profiles separate personal and team settings"
Enter
Sleep 2s

# Hold final frame
Sleep 10s
