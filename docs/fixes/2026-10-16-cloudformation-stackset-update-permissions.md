# Preserve StackSet permission models on update

StackSet updates omitted the configured permission model from the AWS request.
Updates now pass `SELF_MANAGED` or `SERVICE_MANAGED`, matching creation and
allowing CloudFormation to validate any model transition. AWS errors continue
to propagate to the caller.

Regression tests inspect the update request for both permission models and
verify successful operation polling. The existing API-error test covers error
propagation.
