#!/bin/bash
security_account_name="security";

cat test.json \
       | jq -c ".full_account_map.value
       | to_entries
       | map(select(.key==\"${security_account_name}\"|not)
       |{AccountId: .value})"

       aws securityhub add-menber --members 