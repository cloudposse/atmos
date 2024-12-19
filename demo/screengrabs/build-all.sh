#!/bin/bash
set -e
export TERM=xterm-256color

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
  sed -i '' '/- Finding latest version of/d' $file
  sed -i '' '/- Installed hashicorp/d' $file
  sed -i '' '/- Installing hashicorp/d' $file
  sed -i '' '/Terraform has created a lock file/d' $file
  sed -i '' '/Include this file in your version control repository/d' $file
  sed -i '' '/guarantee to make the same selections by default when/d' $file
  sed -i '' '/you run "terraform init" in the future/d' $file
  sed -i '' 's/Resource actions are indicated with the following symbols.*//' $file
  sed -i '' 's/^ *EOT/\n/g' $file
  sed -i '' 's/ *<<EOT/\n/g' $file
  sed -i '' 's/ *<<-EOT/\n/g' $file
  sed -i '' -E 's/\[id=[a-f0-9]+\]//g' $file
  sed -i '' -E 's/(\[id=http)/\n    \1/g' $file
}

postprocess_html() {
  local file=$1
  sed -i '' 's/color:blue/color:#005f87/g' $file
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
