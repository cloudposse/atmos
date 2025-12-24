# env scene - Show environment variables for authentication
Output auth-env.gif
Output auth-env.mp4
Output auth-env.svg

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
Type "# Export auth credentials as environment variables"
Enter
Sleep 1s

Type "atmos auth env"
Sleep 2s
Enter
Sleep 3s

# Capture screenshot
Screenshot auth-env.png

Type "# Use with eval to set credentials in current shell"
Enter
Sleep 2s

Type "eval $(atmos auth env)"
Sleep 2s
Enter
Sleep 2s

Type "# Credentials now available for terraform and AWS CLI"
Enter
Sleep 1s


# Hold final frame
Sleep 10s
