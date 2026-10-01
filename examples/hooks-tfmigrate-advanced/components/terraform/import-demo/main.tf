# Simulates a value that was generated/recorded outside Terraform (e.g. a
# token minted by another system) and needs to be brought under management
# without regenerating it.
resource "random_string" "imported" {
  length = 8
}

output "imported_value" {
  value = random_string.imported.result
}
