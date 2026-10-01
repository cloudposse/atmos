output "resource_group_name" {
  description = "Name of the environment resource group."
  value       = azurerm_resource_group.this.name
}

output "virtual_network_name" {
  description = "Name of the environment virtual network."
  value       = azurerm_virtual_network.this.name
}

output "subnet_id" {
  description = "ID of the default subnet."
  value       = azurerm_subnet.default.id
}
