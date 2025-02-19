#!/bin/bash
set -e
export TERM=xterm-256color

# Ensure that the output is not paginated
export LESS=-X

# Determine the correct sed syntax based on the operating system
if [ "$(uname)" = "Darwin" ]; then
		SED="$SED" # macOS requires '' for in-place editing
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
    if [ "$(uname)" = "Darwin" ]; then
        # macOS-specific syntax
        if [ "${extension}" == "sh" ]; then
            script -q $output_ansi command $command > /dev/null
        else
            script -q $output_ansi bash -c "cd $demo_path && ($command)" > /dev/null
        fi
    else
        # Linux-specific syntax
        if [ "${extension}" = "sh" ]; then
            script -q -a $output_ansi -c "$command" > /dev/null
        else
            script -q -a $output_ansi -c "cd $demo_path && ($command)" > /dev/null
        fi
    fi
    postprocess_ansi $output_ansi
    aha --no-header < $output_ansi > $output_html
    postprocess_html $output_html
    rm -f $output_ansi
    if [ -n "$CI" ]; then
        sed -i -e '1,1d' -e '$d' $output_html
    fi
}

postprocess_ansi() {
  local file=$1
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
  $SED 's/color:blue/color:#005f87/g' $file
	$SED 's/color:#183691/color:#005f87/g' $file
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
