
# Guidelines for Printing Usage and Help Information in Atmos

## Abstract

This document defines the standards for printing **usage** and **help** information in Atmos CLI. It distinguishes between "usage" and "help," clarifies their purposes, and outlines best practices for implementation.

---

## Definitions

### Usage

- **Definition**: A *concise* summary of the correct way to use a specific command or subcommand. It should not show any other helpful information, or examples.
- **Purpose**:
  - To provide quick guidance on the syntax and required arguments when a command is misused.
  - To assist users in resolving errors without overwhelming them with excessive details.
- **Scope**:
  - Usage information is displayed only in error scenarios or upon user request for a brief syntax reminder.

### Help

- **Definition**: A detailed explanation of a command or subcommand, including all available options, parameters, and examples.
- **Purpose**:
  - To serve as a comprehensive resource for users seeking to understand all features and capabilities of a command.
- **Scope**:
  - Help includes usage information but also extends beyond it with detailed documentation.

---

## Usage Guidelines

1. **When to Display Usage**:
   - Display when a command is misused, such as:
     - Missing required arguments.
     - Invalid arguments provided.
     - Syntax errors in the command.
   - Example:

     ```plaintext
     Error: Missing required argument `--stack`.
     Usage:
         atmos terraform plan --stack <stack-name>
     ```

2. **Behavior**:
   - **Error Context**: Always display usage after a clear error message.
   - **Brevity**: Usage output must be concise and limited to the essential syntax.
   - **Exit Code**: Displaying usage must result in a **non-zero exit code**.
   - **No Logo**: The usage output must never include the Atmos logo or branding.

3. **Output Characteristics**:
   - Usage must clearly indicate:
     - Command syntax.
     - Required and optional arguments.
     - Minimal explanation to guide correction of misuse.

---

## Help Guidelines

1. **When to Display Help**:
   - Display when the user explicitly requests it, such as:
     - Running the `help` subcommand:

       ```bash
       atmos help
       ```

     - Using the `--help` flag with a command:

       ```bash
       atmos terraform --help
       ```

2. **Behavior**:
   - **Comprehensive**: Help output must include:
     - A description of the command.
     - All available options, flags, and parameters.
     - Examples of usage.
   - **Error-Free**:
     - Help must always succeed and exit with a **zero exit code**.
     - Help should not fail due to misconfigured files, missing dependencies, or validation errors.
   - **Static**:
     - Help must be non-interactive and preformatted.

3. **Atmos Logo**:
   - The Atmos logo can be included in the `help` output, especially for the main `help` command.
   - Subcommands or options help may omit the logo to keep focus on the content.

4. **No Validation**:
   - Help must not perform any validation, such as:
     - Checking configuration files.
     - Verifying that required tools (e.g., Terraform) are installed.

5. **Example Output**

   ```plaintext
   Atmos CLI - Version 1.0.0

   Usage:
       atmos <command> [options]

   Available Commands:
       terraform    Manage Terraform workflows
       helmfile     Manage Helmfile workflows
       help         Show this help message

   Options:
       -v, --version    Show the Atmos version
       -h, --help       Show help for a command

   Examples:

   - Display the current atmos version

     $ atmos version

   - Display the help for the terraform subcommand

     $ atmos terraform --help

   ```

---

## Comparison of Usage and Help

| Feature                 | Usage                                   | Help                               |
| ----------------------- | --------------------------------------- | ---------------------------------- |
| **Purpose**             | Quick syntax guidance after an error    | Comprehensive documentation        |
| **When Displayed**      | After a command misuse or error         | Explicitly requested by the user   |
| **Content**             | Syntax and required arguments only      | Detailed descriptions and examples |
| **Exit Code**           | Non-zero (when accompanied by an error) | Zero                               |
| **Includes Logo**       | No                                      | Yes                                |
| **Triggers Validation** | No                                      | No                                 |

---

## Implementation Notes

1. **Always Return Expected Output**:
   - Usage for errors.
   - Help for guidance.
2. **Clarity First**:
   - Separate error messages from usage or help outputs clearly.
3. **Testing Requirements**:
   - Ensure usage and help outputs work as defined under all conditions.
4. **Avoid Redundancy**:
   - Help includes usage; there is no need for separate outputs in `help` documentation.
