# Generic test component for YAML function testing
# This component is designed to test various data types and scenarios
# without imposing specific cloud provider conventions

# Scalar types
variable "test_string" {
  description = "Test string variable"
  type        = string
  default     = "default-string"
}

variable "test_number" {
  description = "Test number variable"
  type        = number
  default     = 42
}

variable "test_bool" {
  description = "Test boolean variable"
  type        = bool
  default     = false
}

# List types
variable "string_list" {
  description = "List of strings for testing"
  type        = list(string)
  default     = []
}

variable "number_list" {
  description = "List of numbers for testing"
  type        = list(number)
  default     = []
}

variable "mixed_type_list" {
  description = "List that can contain different types (as strings)"
  type        = list(string)
  default     = []
}

# Map types
variable "string_map" {
  description = "Map of strings for testing"
  type        = map(string)
  default     = {}
}

variable "number_map" {
  description = "Map of numbers for testing"
  type        = map(number)
  default     = {}
}

# Complex nested structures
variable "nested_object" {
  description = "Nested object for complex testing"
  type = object({
    name   = string
    count  = number
    active = bool
    tags   = list(string)
    metadata = map(string)
  })
  default = {
    name     = "test-object"
    count    = 1
    active   = true
    tags     = []
    metadata = {}
  }
}

# Lists of maps (common pattern)
variable "list_of_maps" {
  description = "List of maps for testing complex structures"
  type = list(map(string))
  default = []
}

# Test scenarios for YAML functions in lists
variable "function_results_list" {
  description = "List to be populated by YAML function results"
  type        = list(string)
  default     = []
}

variable "mixed_function_list" {
  description = "List mixing static values and function results"
  type        = list(string)
  default     = []
}

variable "function_map_results" {
  description = "Map to be populated by YAML function results"
  type        = map(string)
  default     = {}
}
