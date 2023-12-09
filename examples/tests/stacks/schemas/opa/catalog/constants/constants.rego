package atmos.constants

vpc_dev_max_availability_zones_error_message := "In 'dev', only 2 Availability Zones are allowed"

vpc_prod_map_public_ip_on_launch_error_message := "Mapping public IPs on launch is not allowed in 'prod'. Set 'map_public_ip_on_launch' variable to 'false'"

vpc_name_regex := "^[a-zA-Z0-9]{2,20}$"

vpc_name_regex_error_message := "VPC name must be a valid string from 2 to 20 alphanumeric chars"
