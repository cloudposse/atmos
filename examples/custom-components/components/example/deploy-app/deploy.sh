#!/bin/bash
# Example deploy script.
#
# Values from the component `env` section are exported as real environment variables, so a script
# invoked from a custom command step can read them directly (e.g. $APP_VERSION, $DEPLOY_REGION).
# Use `!secret NAME` in the stack `env` section for sensitive values — they resolve from a secret
# backend and are masked in output. See https://atmos.tools/cli/configuration/secrets

echo "Deploying ${APP_NAME} version ${APP_VERSION} to ${DEPLOY_REGION}"
