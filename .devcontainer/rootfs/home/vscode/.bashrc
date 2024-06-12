eval "$(direnv hook bash)"
export DIRENV_LOG_FORMAT=""
direnv allow /workspace/examples

find /workspace/examples -type d -exec bash -c 'echo show_readme > {}/.envrc' \;
find /workspace/examples -type d -exec direnv allow {} \;

atmos version
