#!/bin/bash
set -e
export TERM=xterm-256color

# Force color output for screengrabs
export ATMOS_FORCE_COLOR=true
export FORCE_COLOR=1
export CLICOLOR_FORCE=1

# Ensure that the output is not paginated
export LESS=-X
export ATMOS_PAGER=false

# Determine the correct sed syntax based on the operating system
if [ "$(uname)" = "Darwin" ]; then
		SED="sed -i ''" # macOS requires '' for in-place editing
else
		SED="sed -i"    # Linux does not require ''
fi

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
        $SED -e '1,1d' -e '$d' $output_html
    fi
}

postprocess_ansi() {
  local file=$1
  # Remove terminal escape sequences (OSC, cursor position queries, etc.)
  $SED -E 's/\^\[\]([0-9]+;[^\^]*)\^G//g' $file
  $SED -E 's/\^\[(\[[0-9;]+R)//g' $file

  # Remove noise and clean up the output
  $SED '/- Finding latest version of/d' $file
  $SED '/- Installed hashicorp/d' $file
  $SED '/- Installing hashicorp/d' $file
  $SED '/Terraform has created a lock file/d' $file
  $SED '/Include this file in your version control repository/d' $file
  $SED '/guarantee to make the same selections by default when/d' $file
  $SED '/you run "terraform init" in the future/d' $file
  $SED 's/Resource actions are indicated with the following symbols.*//' $file
	$SED '/Workspace .* doesn.t exist./d' $file
	$SED '/You can create this workspace with the .* subcommand/d' $file
	$SED '/or include the .* flag with the .* subcommand./d' $file

  $SED 's/^ *EOT/\n/g' $file
  $SED 's/ *<<EOT/\n/g' $file
  $SED 's/ *<<-EOT/\n/g' $file
  $SED -E 's/\[id=[a-f0-9]+\]//g' $file
  $SED -E 's/(\[id=http)/\n    \1/g' $file
}

postprocess_html() {
  local file=$1
  # Replace blue colors with Atmos blue.
  $SED 's/color:blue/color:#005f87/g' $file
	$SED 's/color:#183691/color:#005f87/g' $file

	# Strip all background colors - they cause visibility issues.
	# aha adds background-color to code blocks which makes text invisible when colors match.
	$SED 's/background-color:[^;]*;//g' $file
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
