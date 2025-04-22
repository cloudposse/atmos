# Example Terraform IPinfo Component

This Terraform module retrieves data from the IPinfo API for a specified IP address. If no IP address is specified, it retrieves data for the requester's IP address.

## Usage

### Inputs

- `ip_address` (optional): The IP address to retrieve information for. If not specified, the requester's IP address will be used. The default value is an empty string.

### Outputs

- `metadata`: The data retrieved from IPinfo for the specified IP address, in JSON format.
