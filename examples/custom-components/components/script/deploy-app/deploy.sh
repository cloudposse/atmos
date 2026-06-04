#!/bin/bash
# Example deploy script - not actually executed by the custom command
# The custom command uses the component vars via Go templates

echo "Deploying ${APP_NAME} version ${VERSION} with ${REPLICAS} replicas"
