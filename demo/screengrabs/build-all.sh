#!/bin/bash
set -e
export TERM=xterm-256color

# Check for required dependencies
MISSING_DEPS=()

if ! command -v aha &> /dev/null; then
    MISSING_DEPS+=("aha")
fi

if ! command -v atmos &> /dev/null; then
    MISSING_DEPS+=("atmos")
fi

if [ ${#MISSING_DEPS[@]} -ne 0 ]; then
    echo "ERROR: Missing required dependencies: ${MISSING_DEPS[*]}" >&2
    echo "" >&2
    echo "Please install the missing dependencies:" >&2
    for dep in "${MISSING_DEPS[@]}"; do
        case "$dep" in
            aha)
                if [ "$(uname)" = "Darwin" ]; then
                    echo "  - aha: Run 'brew bundle' in demo/screengrabs directory" >&2
                else
                    echo "  - aha: Install with 'apt-get install aha' (Debian/Ubuntu)" >&2
                    echo "    See: https://github.com/theZiz/aha" >&2
                fi
                ;;
            atmos)
                echo "  - atmos: Build with 'make build' from the repository root" >&2
                echo "    Or install from: https://atmos.tools/install" >&2
                ;;
        esac
    done
    echo "" >&2
    echo "Alternatively, use Docker to generate screengrabs:" >&2
    echo "  make -C demo/screengrabs docker-all" >&2
    exit 1
fi

# Force color output for screengrabs
export ATMOS_FORCE_COLOR=true
export FORCE_COLOR=1
export CLICOLOR_FORCE=1

# Ensure that the output is not paginated
export LESS=-X
export ATMOS_PAGER=false

# Determine the correct sed syntax based on the operating system
# Function to call sed with proper in-place editing syntax
function sed_inplace() {
	if [ "$(uname)" = "Darwin" ]; then
		sed -i '' "$@"  # macOS requires '' for in-place editing
	else
		sed -i "$@"     # Linux does not require ''
	fi
}

function record() {
    local demo=$1
    local command=$2
    local extension="${command##*.}" # if any...
    local demo_path=../../examples/$demo
    local output_base_file=artifacts/$(echo "$command" | sed -E 's/ -/-/g' | sed -E 's/ +/-/g' | sed 's/---/--/g' | sed 's/scripts\///' | sed 's/\.sh$//')
    local output_html=${output_base_file}.html
    local output_ansi=${output_base_file}.ansi
    local output_dir=$(dirname $output_base_file)

    echo "Screengrabbing $command → $output_html"
    mkdir -p "$output_dir"
    rm -f $output_ansi

    # Direct command execution with ATMOS_FORCE_COLOR (no need for script command)
    if [ "${extension}" = "sh" ]; then
        $command > $output_ansi 2>&1
    else
        (cd $demo_path && $command > "$OLDPWD/$output_ansi" 2>&1)
    fi

    postprocess_ansi $output_ansi
    aha --no-header < $output_ansi > $output_html
    postprocess_html $output_html
    rm -f $output_ansi
    if [ -n "$CI" ]; then
        sed_inplace -e '1,1d' -e '$d' $output_html
    fi
}

postprocess_ansi() {
  local file=$1
  # Remove terminal escape sequences (OSC, cursor position queries, etc.)
  sed_inplace -E 's/\^\[\]([0-9]+;[^\^]*)\^G//g' $file
  sed_inplace -E 's/\^\[(\[[0-9;]+R)//g' $file

  # Remove noise and clean up the output
  sed_inplace '/- Finding latest version of/d' $file
  sed_inplace '/- Installed hashicorp/d' $file
  sed_inplace '/- Installing hashicorp/d' $file
  sed_inplace '/Terraform has created a lock file/d' $file
  sed_inplace '/Include this file in your version control repository/d' $file
  sed_inplace '/guarantee to make the same selections by default when/d' $file
  sed_inplace '/you run "terraform init" in the future/d' $file
  sed_inplace 's/Resource actions are indicated with the following symbols.*//' $file
	sed_inplace '/Workspace .* doesn.t exist./d' $file
	sed_inplace '/You can create this workspace with the .* subcommand/d' $file
	sed_inplace '/or include the .* flag with the .* subcommand./d' $file

  sed_inplace 's/^ *EOT/\n/g' $file
  sed_inplace 's/ *<<EOT/\n/g' $file
  sed_inplace 's/ *<<-EOT/\n/g' $file
  sed_inplace -E 's/\[id=[a-f0-9]+\]//g' $file
  sed_inplace -E 's/(\[id=http)/\n    \1/g' $file
}

postprocess_html() {
  local file=$1
  # Replace blue colors with Atmos blue.
  sed_inplace 's/color:blue/color:#005f87/g' $file
	sed_inplace 's/color:#183691/color:#005f87/g' $file

	# Strip all background colors - they cause visibility issues.
	# aha adds background-color to code blocks which makes text invisible when colors match.
	sed_inplace 's/background-color:[^;]*;//g' $file
}

manifest=$1

if [ -z "$manifest" ]; then
    echo "Usage: $0 <manifest>"
    exit 1
fi

demo=$(basename $manifest .txt)

index=0
while IFS= read -r command; do
    commands[index++]="$command"
done < <(grep -v '^#' "$manifest")

for command in "${commands[@]}"; do
    record "$demo" "$command"
done
