#!/bin/bash

relative_path=$(dirname `realpath "$0"`)
source $relative_path/.demo.rc
clean

comment "Now, let's see what it looks like to deploy this everywhere! 🚀"
comment ""
comment ""
$relative_path/deploy-dev.sh

newline 5
$relative_path/deploy-staging.sh

newline 5
$relative_path/deploy-prod.sh
