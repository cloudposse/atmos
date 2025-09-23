#!/usr/bin/env python3

import os

base_path = "/Users/erik/Dev/cloudposse/tools/atmos/.conductor/custom-planfile/website/docs/cli/commands/terraform"

# Define updates for each command
updates = {
    "terraform-fmt.mdx": {
        "purpose_addon": " This command is a direct passthrough to the native `terraform fmt` command.",
        "info_box": "\n\n:::info Atmos Behavior\nThis is a pure passthrough command. Atmos does not perform automatic initialization, workspace management, or variable generation for `terraform fmt`. The command is executed directly in the component directory.\n:::"
    },
    "terraform-version.mdx": {
        "purpose_addon": " This command is a direct passthrough to the native `terraform version` command.",
        "info_box": "\n\n:::info Atmos Behavior\nThis is a pure passthrough command. Atmos does not perform any special processing for `terraform version`. The command is executed directly without workspace management or variable generation.\n:::"
    },
    "terraform-login.mdx": {
        "purpose_addon": "",
        "info_box": "\n\n:::info Atmos Behavior\nThis is a pure passthrough command. Atmos does not perform automatic initialization or workspace management for `terraform login`. The command is executed directly to manage Terraform Cloud credentials.\n:::"
    },
    "terraform-logout.mdx": {
        "purpose_addon": "",
        "info_box": "\n\n:::info Atmos Behavior\nThis is a pure passthrough command. Atmos does not perform automatic initialization or workspace management for `terraform logout`. The command is executed directly to manage Terraform Cloud credentials.\n:::"
    },
    "terraform-console.mdx": {
        "purpose_addon": "",
        "info_box": "\n\n:::info Atmos Behavior\nAtmos provides standard setup for this command including automatic `terraform init`, workspace selection, and variable file generation. The console itself operates as native Terraform.\n:::"
    },
    "terraform-output.mdx": {
        "purpose_addon": "",
        "info_box": "\n\n:::info Atmos Behavior\nAtmos provides standard setup for this command including automatic `terraform init`, workspace selection, and variable file generation. The output retrieval itself is handled by native Terraform.\n:::"
    },
    "terraform-show.mdx": {
        "purpose_addon": "",
        "info_box": "\n\n:::info Atmos Behavior\nAtmos provides standard setup for this command including automatic `terraform init` and workspace selection. The show operation itself is handled by native Terraform.\n:::"
    },
    "terraform-providers.mdx": {
        "purpose_addon": "",
        "info_box": "\n\n:::info Atmos Behavior\nAtmos provides standard setup for this command including automatic `terraform init` and workspace selection. The provider information display is handled by native Terraform.\n:::"
    },
    "terraform-graph.mdx": {
        "purpose_addon": "",
        "info_box": "\n\n:::info Atmos Behavior\nAtmos provides standard setup for this command including automatic `terraform init` and workspace selection. The graph generation itself is handled by native Terraform.\n:::"
    },
    "terraform-get.mdx": {
        "purpose_addon": "",
        "info_box": "\n\n:::info Atmos Behavior\nAtmos provides standard setup for this command including automatic `terraform init` and workspace selection. The module download operation is handled by native Terraform.\n:::"
    },
    "terraform-force-unlock.mdx": {
        "purpose_addon": "",
        "info_box": "\n\n:::info Atmos Behavior\nAtmos provides standard setup for this command including automatic `terraform init` and workspace selection. The unlock operation itself is handled by native Terraform.\n:::"
    },
    "terraform-test.mdx": {
        "purpose_addon": "",
        "info_box": "\n\n:::info Atmos Behavior\nAtmos provides standard setup for this command including automatic `terraform init` and workspace selection. The test execution is handled by native Terraform.\n:::"
    },
    "terraform-metadata.mdx": {
        "purpose_addon": "",
        "info_box": "\n\n:::info Atmos Behavior\nAtmos provides standard setup for this command including automatic `terraform init` and workspace selection. The metadata operations are handled by native Terraform.\n:::"
    },
    "terraform-modules.mdx": {
        "purpose_addon": "",
        "info_box": "\n\n:::info Atmos Behavior\nAtmos provides standard setup for this command including automatic `terraform init` and workspace selection. The module listing is handled by native Terraform.\n:::"
    },
    "terraform-validate.mdx": {
        "purpose_addon": "",
        "info_box": "\n\n:::info Atmos Behavior\nAtmos provides standard setup for this command including automatic `terraform init` and workspace selection. The validation itself is performed by native Terraform.\n:::"
    },
    "terraform-state.mdx": {
        "purpose_addon": "",
        "info_box": "\n\n:::info Atmos Behavior\nAtmos provides standard setup for this command including automatic `terraform init` and workspace selection. Additionally, Atmos blocks state modifications if the component is locked (`metadata.locked: true`). The state operations themselves are handled by native Terraform.\n:::"
    },
    "terraform-taint.mdx": {
        "purpose_addon": "",
        "info_box": "\n\n:::info Atmos Behavior\nAtmos provides standard setup for this command including automatic `terraform init`, workspace selection, and variable file generation. Additionally, Atmos blocks this operation if the component is locked (`metadata.locked: true`). The taint operation itself is handled by native Terraform.\n:::"
    },
    "terraform-untaint.mdx": {
        "purpose_addon": "",
        "info_box": "\n\n:::info Atmos Behavior\nAtmos provides standard setup for this command including automatic `terraform init`, workspace selection, and variable file generation. Additionally, Atmos blocks this operation if the component is locked (`metadata.locked: true`). The untaint operation itself is handled by native Terraform.\n:::"
    },
    "terraform-import.mdx": {
        "purpose_addon": "",
        "info_box": "\n\n:::info Atmos Enhancements\nAtmos provides several enhancements for the import command:\n- Automatic `terraform init` before importing\n- Workspace selection and management\n- Variable file generation and passing\n- **Automatic AWS_REGION environment variable** setting based on the component's region variable\n- Component locking support (blocks import if `metadata.locked: true`)\n:::"
    },
    "terraform-refresh.mdx": {
        "purpose_addon": "",
        "info_box": "\n\n:::info Atmos Enhancements\nAtmos enhances the refresh command with:\n- Automatic `terraform init` before refreshing\n- Workspace selection and management\n- **Automatic variable file generation and passing**\n- Backend configuration\n- Component validation\n:::"
    },
    "terraform-destroy.mdx": {
        "purpose_addon": "",
        "info_box": "\n\n:::info Atmos Enhancements\nAtmos enhances the destroy command with:\n- Automatic `terraform init` before destroying\n- Workspace selection and management\n- **Automatic variable file generation and passing**\n- Backend configuration\n- Component validation and locking support\n:::"
    }
}

def update_file(filename, updates_dict):
    filepath = os.path.join(base_path, filename)
    if not os.path.exists(filepath):
        print(f"  ⚠️  File not found: {filename}")
        return False

    with open(filepath, 'r') as f:
        content = f.read()

    original_content = content

    # Add purpose addon if specified
    if updates_dict.get("purpose_addon"):
        content = content.replace(
            ":::note purpose\nUse this command",
            f":::note purpose\nUse this command"
        )
        # Add at end of purpose note
        content = content.replace(
            ":::\n\n<Screengrab",
            f"{updates_dict['purpose_addon']}\n:::\n\n<Screengrab"
        )

    # Add info box after the description paragraph
    if updates_dict.get("info_box"):
        # Find the line that starts with "This command" and add the info box after that paragraph
        lines = content.split('\n')
        new_lines = []
        added_info = False

        for i, line in enumerate(lines):
            new_lines.append(line)

            # Look for the end of the description paragraph (empty line after "This command...")
            if not added_info and i > 0 and lines[i-1].startswith("This command") and line == "":
                new_lines.append(updates_dict["info_box"].strip())
                added_info = True

        if added_info:
            content = '\n'.join(new_lines)

    if content != original_content:
        with open(filepath, 'w') as f:
            f.write(content)
        return True
    return False

# Apply updates
print("Updating Terraform documentation files...")
updated_count = 0
for filename, updates_dict in updates.items():
    print(f"Checking {filename}...")
    if update_file(filename, updates_dict):
        print(f"  ✅ Updated {filename}")
        updated_count += 1
    else:
        print(f"  ⏭️  No changes needed for {filename}")

print(f"\n✅ Updated {updated_count} files")

# Clean up the analysis file
analysis_file = "/Users/erik/Dev/cloudposse/tools/atmos/.conductor/custom-planfile/terraform_command_analysis.md"
if os.path.exists(analysis_file):
    os.remove(analysis_file)
    print(f"🧹 Removed temporary analysis file")
