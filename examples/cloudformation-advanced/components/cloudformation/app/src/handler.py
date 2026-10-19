"""A small HTTP application whose release is compiled into its package."""

import json

from release import RELEASE


def handler(event, context):
    return {
        "statusCode": 200,
        "headers": {"Content-Type": "application/json"},
        "body": json.dumps({"message": "hello from Atmos", "release": RELEASE}),
    }
