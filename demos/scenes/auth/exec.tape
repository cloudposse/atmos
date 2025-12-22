# exec-auth scene - Execute commands with team credentials
Output auth-exec.gif
Output auth-exec.mp4

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
Type "# Execute commands with team credentials"
Enter
Sleep 1s

Type "# Run any command with assumed role"
Enter
Sleep 500ms

Type "atmos auth exec -- aws sts get-caller-identity"
Sleep 500ms
Enter
Sleep 3s

# Capture screenshot
Screenshot auth-exec.png

Type "# Access cloud resources securely"
Enter
Sleep 500ms

Type "atmos auth exec -- kubectl get pods"
Sleep 500ms
Enter
Sleep 3s

Type "# Unified credential management"
Enter
Sleep 1s
