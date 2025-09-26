#!/bin/bash
set -e

# List of all terraform commands that need screengrabs
commands=(
    "atmos terraform apply --help"
    "atmos terraform console --help"
    "atmos terraform destroy --help"
    "atmos terraform fmt --help"
    "atmos terraform force-unlock --help"
    "atmos terraform get --help"
    "atmos terraform graph --help"
    "atmos terraform import --help"
    "atmos terraform init --help"
    "atmos terraform login --help"
    "atmos terraform logout --help"
    "atmos terraform metadata --help"
    "atmos terraform modules --help"
    "atmos terraform output --help"
    "atmos terraform plan --help"
    "atmos terraform plan-diff --help"
    "atmos terraform providers --help"
    "atmos terraform refresh --help"
    "atmos terraform show --help"
    "atmos terraform state --help"
    "atmos terraform taint --help"
    "atmos terraform test --help"
    "atmos terraform untaint --help"
    "atmos terraform validate --help"
    "atmos terraform version --help"
)

# Ensure we have the atmos binary built
if [ ! -f "../../build/atmos" ]; then
    echo "Error: atmos binary not found at ../../build/atmos"
    echo "Please run 'make build' first"
    exit 1
fi

# Use the local built atmos
export PATH="../../build:$PATH"

echo "Generating terraform command screengrabs..."

for cmd in "${commands[@]}"; do
    # Convert command to filename format (atmos terraform command --help -> atmos-terraform-command--help)
    slug=$(echo "$cmd" | sed 's/ --help/--help/' | sed 's/ /-/g')
    filename="${slug}.html"
    output_path="../../website/src/components/Screengrabs/${filename}"

    # Skip if already exists and not forcing regeneration
    if [ -f "$output_path" ] && [ "$1" != "--force" ]; then
        echo "✓ Skipping $filename (already exists)"
        continue
    fi

    echo "Generating $filename..."

    # Run the command and capture ANSI output
    temp_ansi="/tmp/${slug}.ansi"
    temp_html="/tmp/${slug}.html"

    # Execute the command and capture output with ANSI colors
    TERM=xterm-256color $cmd 2>&1 | tee "$temp_ansi" > /dev/null || true

    # Convert ANSI to HTML using aha
    if command -v aha &> /dev/null; then
        aha --no-header < "$temp_ansi" > "$temp_html"
    else
        echo "Warning: 'aha' not installed. Install with: brew install aha"
        # Fallback: just escape HTML entities
        sed 's/&/\&amp;/g; s/</\&lt;/g; s/>/\&gt;/g' "$temp_ansi" > "$temp_html"
    fi

    # Post-process the HTML
    # Remove script header/footer if present
    if [ "$(uname)" = "Darwin" ]; then
        sed -i '' '1s/^.*<pre>//' "$temp_html" 2>/dev/null || true
        sed -i '' '$s/<\/pre>.*$//' "$temp_html" 2>/dev/null || true
    else
        sed -i '1s/^.*<pre>//' "$temp_html" 2>/dev/null || true
        sed -i '$s/<\/pre>.*$//' "$temp_html" 2>/dev/null || true
    fi

    # Copy to final location
    cp "$temp_html" "$output_path"
    echo "✓ Generated $filename"

    # Clean up temp files
    rm -f "$temp_ansi" "$temp_html"
done

echo "Done! Generated screengrabs for terraform commands."
echo "Files are in: website/src/components/Screengrabs/"
