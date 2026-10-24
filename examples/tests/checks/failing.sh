#!/bin/sh
# A deliberate failure makes the failure-only output behavior visible.
echo 'Checking the authentication response...'
sleep 0.5
echo 'Expected HTTP 200; received HTTP 503.' >&2
exit 1
