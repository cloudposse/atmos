# Git Ignore Template

This template provides a comprehensive `.gitignore` file for infrastructure and development projects.

## What is .gitignore?

The `.gitignore` file tells Git which files and directories to ignore when tracking changes. This is essential for keeping your repository clean by excluding:
- Build artifacts and compiled files
- Dependencies and vendor directories
- IDE and editor-specific files
- Log files and temporary data
- Sensitive configuration files

## Usage

Run the following command to initialize the Git ignore file:

```bash
atmos init .gitignore
```

This will create a `.gitignore` file in the current directory with comprehensive ignore patterns for:
- Terraform state files and lock files
- Provider plugins and modules
- IDE files (VS Code, IntelliJ, etc.)
- OS-specific files (macOS, Windows, Linux)
- Build artifacts and temporary files
- Log files and debugging output

## Customization

After initialization, you can modify the `.gitignore` file to add or remove patterns based on your specific project needs and technology stack.
