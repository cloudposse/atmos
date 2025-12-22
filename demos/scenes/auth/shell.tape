# shell scene - Start interactive shell with credentials
Output auth-shell.gif
Output auth-shell.mp4

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
Type "# Start interactive shell with credentials"
Enter
Sleep 1s

Type "# Launch shell with AWS credentials injected"
Enter
Sleep 500ms

Type "atmos auth shell"
Sleep 500ms
Enter
Sleep 3s

# Capture screenshot
Screenshot auth-shell.png

Type "# Credentials are automatically available"
Enter
Sleep 500ms

Type "aws sts get-caller-identity"
Sleep 500ms
Enter
Sleep 3s

Type "exit"
Enter
Sleep 1s
