# Local equals CI

Source demo/landing/defaults.tape

Output website/static/img/demos/local-ci.webm
Output website/static/img/demos/local-ci.mp4

Hide
Type "unset NO_COLOR ATMOS_NO_COLOR" Enter
Type `export ATMOS_EXPERIMENTAL=silence ATMOS_TERMINAL_SPEED=8 ATMOS_PAGER=false NO_PAGER=1 GIT_PAGER=cat TF_INPUT=false TERM=xterm-256color COLORTERM=truecolor ATMOS_FORCE_COLOR=true FORCE_COLOR=1 CLICOLOR_FORCE=1 PS1='\[\033[38;5;42m\]❯\[\033[0m\]'` Enter
Type `export ATMOS_COMPONENTS_TERRAFORM_APPLY_AUTO_APPROVE=true` Enter
Type `repo="$(git rev-parse --show-toplevel)"` Enter
Type `work="$(mktemp -d)"` Enter
Type `export CI=true` Enter
Type `rsync -a --exclude '.terraform' --exclude '*.terraform.tfvars.json' --exclude 'terraform.tfstate*' "$repo/demo/landing/fixtures/local-ci/" "$work/"` Enter
Type `cd "$work"` Enter
Type `git init -q && git config user.email demo@atmos.tools && git config user.name demo && git config commit.gpgsign false && git add -A && git commit -qm baseline` Enter
Wait /❯$/
Type `git branch baseline` Enter
Wait /❯$/
Type `atmos terraform workspace new myapp -s dev` Enter
Wait /❯$/
Type "clear" Enter
Wait /❯$/
Show

Sleep 800ms
Type `perl -i -pe 's/location: Stockholm/location: Berlin/' stacks/deploy/dev.yaml` Sleep 400ms Enter
Wait /❯$/
Type "git diff stacks/deploy/dev.yaml" Sleep 400ms Enter
Wait /❯$/
Sleep 2.5s
Type `git commit -aqm 'dev: move region to Berlin'` Sleep 400ms Enter
Wait /❯$/

Type `atmos list affected --ref baseline` Sleep 600ms Enter
Wait /❯$/
Sleep 4s
Type `atmos terraform apply --affected --ref baseline --ci` Sleep 600ms Enter
Wait /❯$/
Sleep 3s

Screenshot website/static/img/demos/local-ci.png
Sleep 5s

Hide
Type `cd "$repo" && rm -rf "$work"` Enter
Wait /❯$/
