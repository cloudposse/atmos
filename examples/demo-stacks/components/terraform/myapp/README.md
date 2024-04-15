# Example Terraform Weather Component

This Terraform "root" module fetches weather information for a specified location with custom display options.
It queries data from the [`wttr.in`](https://wttr.in) weather service and stores the result in a local file (`cache.txt`). 
It also provides several outputs like weather information, request URL, stage, location, language, and units of measurement.

## Features

- Fetch weather updates for a location using HTTP request.
- Write the obtained weather data in a local file.
- Customizable display options.
- View the request URL.
- Get informed about the stage, location, language, and units in the metadata.

## Usage

To include this module in your [Atmos Stacks](https://atmos.tools/core-concepts/stacks) configuration:

```yaml
components:
  terraform:
    weather:
      vars:
        stage: dev
        location: New York
        options: 0T
        format: v2
        lang: en
        units: m
```

### Inputs
- `stage`: Stage where it will be deployed.
- `location`: Location for which the weather is reported. Default is "Los Angeles".
- `options`: Options to customize the output. Default is "0T".
- `format`: Specifies the output format. Default is "v2".
- `lang`: Language in which the weather will be displayed. Default is "en".
- `units`: Units in which the weather will be displayed. Default is "m".

### Outputs
- `weather`: The fetched weather data.
- `url`: Requested URL.
- `stage`: Stage of deployment.
- `location`: Location of the reported weather.
- `lang`: Language used for weather data.
- `units`: Units of measurement for the weather data.

Please note, this module requires Terraform version >=1.0.0, and you need to specify no other required providers.

Happy Weather Tracking!
