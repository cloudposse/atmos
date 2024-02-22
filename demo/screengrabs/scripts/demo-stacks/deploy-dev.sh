#!/bin/bash

relative_path=$(dirname `realpath "$0"`)
source $relative_path/.demo.rc
clean

comment "Deploy your component to the dev environment 🚀"
prompt
run atmos terraform deploy myapp --stack dev
