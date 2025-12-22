# login scene - Authenticate with cloud providers
Output auth-login.gif
Output auth-login.mp4

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
Type "# Authenticate with cloud providers"
Enter
Sleep 1s

Type "# Check current authentication status"
Enter
Sleep 500ms

Type "atmos auth status"
Sleep 500ms
Enter
Sleep 3s

# Capture screenshot
Screenshot auth-login.png

Type "# Login to AWS, Azure, or other providers"
Enter
Sleep 500ms

Type "atmos auth login"
Sleep 500ms
Enter
Sleep 3s

Type "# Credentials cached for subsequent commands"
Enter
Sleep 1s
