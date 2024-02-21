#!/bin/bash
set -e
export TERM=xterm-256color

function record() {
		demo=$1
    command=$2
		
		demo_path=../../examples/$demo
    output_base_file=artifacts/$(echo "$command" | sed -E 's/ -/-/g' | sed -E 's/ +/-/g' | sed 's/---/--/g' | sed 's/scripts\///' | sed 's/\.sh$//')
		output_html=${output_base_file}.html
		output_ansi=${output_base_file}.ansi
		output_dir=$(dirname $output_base_file)
    echo "Screengrabbing $command → $output_html"
		mkdir -p "$output_dir"
		rm -f $output_ansi
    script -q $output_ansi bash -c "cd $demo_path && ($command)" > /dev/null
		aha --no-header < $output_ansi > $output_html
		rm -f $output_ansi
    if [ -n "$CI" ]; then
        sed -i -e '1,1d' -e '$d' $output_html
    fi
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
