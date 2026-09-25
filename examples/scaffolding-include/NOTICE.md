# atmos:template
# License Notice

This project is licensed under **{{ (index .Config.license_lookup .Config.license).full_name }}**.

See: {{ (index .Config.license_lookup .Config.license).url }}
