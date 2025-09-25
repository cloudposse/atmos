# Outputs for all variable types to enable testing of YAML function returns

# Scalar outputs
output "test_string" {
  description = "String output for testing"
  value       = var.test_string
}

output "test_number" {
  description = "Number output for testing"
  value       = var.test_number
}

output "test_bool" {
  description = "Boolean output for testing"
  value       = var.test_bool
}

# List outputs
output "string_list" {
  description = "String list output"
  value       = var.string_list
}

output "number_list" {
  description = "Number list output"
  value       = var.number_list
}

output "mixed_type_list" {
  description = "Mixed type list output"
  value       = var.mixed_type_list
}

# Map outputs
output "string_map" {
  description = "String map output"
  value       = var.string_map
}

output "number_map" {
  description = "Number map output"
  value       = var.number_map
}

# Complex outputs
output "nested_object" {
  description = "Nested object output"
  value       = var.nested_object
}

output "list_of_maps" {
  description = "List of maps output"
  value       = var.list_of_maps
}

# Function test outputs
output "function_results_list" {
  description = "List populated by YAML functions"
  value       = var.function_results_list
}

output "mixed_function_list" {
  description = "List with mixed static and function values"
  value       = var.mixed_function_list
}

output "function_map_results" {
  description = "Map populated by YAML functions"
  value       = var.function_map_results
}

# Additional outputs for testing specific scenarios
output "all_vars" {
  description = "All variables as a map for comprehensive testing"
  value = {
    string = var.test_string
    number = var.test_number
    bool   = var.test_bool
    lists = {
      strings = var.string_list
      numbers = var.number_list
      mixed   = var.mixed_type_list
    }
    maps = {
      strings = var.string_map
      numbers = var.number_map
    }
  }
}

output "list_count" {
  description = "Count of items in function results list"
  value       = length(var.function_results_list)
}

output "first_item" {
  description = "First item from function results list (if exists)"
  value       = length(var.function_results_list) > 0 ? var.function_results_list[0] : null
}
