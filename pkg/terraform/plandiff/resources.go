package plandiff

// processPriorStateResources extracts resources from prior_state.
func processPriorStateResources(plan map[string]interface{}, result map[string]interface{}) {
	priorState, ok := plan["prior_state"].(map[string]interface{})
	if !ok {
		return
	}

	values, ok := priorState["values"].(map[string]interface{})
	if !ok {
		return
	}

	rootModule, ok := values["root_module"].(map[string]interface{})
	if !ok {
		return
	}

	processRootModuleResources(rootModule, result)
}

// processPlannedValuesResources extracts resources from planned_values.
func processPlannedValuesResources(plan map[string]interface{}, result map[string]interface{}) {
	plannedValues, ok := plan["planned_values"].(map[string]interface{})
	if !ok {
		return
	}

	rootModule, ok := plannedValues["root_module"].(map[string]interface{})
	if !ok {
		return
	}

	processRootModuleResources(rootModule, result)
}

// processRootModuleResources processes resources from a root_module.
func processRootModuleResources(rootModule map[string]interface{}, result map[string]interface{}) {
	resources, ok := rootModule["resources"].([]interface{})
	if !ok {
		return
	}

	for _, res := range resources {
		resMap, ok := res.(map[string]interface{})
		if !ok {
			continue
		}

		addressVal, ok := resMap["address"]
		if !ok {
			continue
		}

		modeVal, ok := resMap["mode"]
		if !ok {
			continue
		}

		mode, ok := modeVal.(string)
		if !ok {
			continue
		}

		if mode == "data" {
			continue
		}

		address, ok := addressVal.(string)
		if !ok {
			continue
		}

		result[address] = resMap
	}
}

// processResourceChanges extracts resources from resource_changes.
func processResourceChanges(plan map[string]interface{}, result map[string]interface{}) {
	resourceChanges, ok := plan["resource_changes"].([]interface{})
	if !ok {
		return
	}

	for _, change := range resourceChanges {
		changeMap, ok := change.(map[string]interface{})
		if !ok {
			continue
		}

		addressVal, ok := changeMap["address"]
		if !ok {
			continue
		}

		modeVal, ok := changeMap["mode"]
		if !ok {
			continue
		}

		mode, ok := modeVal.(string)
		if !ok {
			continue
		}

		if mode == "data" {
			continue
		}

		address, ok := addressVal.(string)
		if !ok {
			continue
		}

		result[address] = changeMap
	}
}

// getVariables extracts variables from a terraform plan.
func getVariables(plan map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	if vars, ok := plan["variables"].(map[string]interface{}); ok {
		for k, v := range vars {
			if varMap, ok := v.(map[string]interface{}); ok {
				if value, exists := varMap["value"]; exists {
					result[k] = value
				}
			}
		}
	}

	return result
}

// getResources extracts resources from a terraform plan.
func getResources(plan map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Process resources from different sections
	processPriorStateResources(plan, result)
	processPlannedValuesResources(plan, result)
	processResourceChanges(plan, result)

	return result
}

// getOutputs extracts outputs from a terraform plan.
func getOutputs(plan map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Check planned_values for outputs
	if plannedValues, ok := plan["planned_values"].(map[string]interface{}); ok {
		if outputs, ok := plannedValues["outputs"].(map[string]interface{}); ok {
			for k, v := range outputs {
				result[k] = v
			}
		}
	}

	// Check output_changes
	if outputChanges, ok := plan["output_changes"].(map[string]interface{}); ok {
		for k, v := range outputChanges {
			if _, exists := result[k]; !exists {
				result[k] = v
			}
		}
	}

	return result
}

// getResourceAttributes extracts attributes from a resource.
func getResourceAttributes(resource interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	resMap, ok := resource.(map[string]interface{})
	if !ok {
		return result
	}

	// Extract values from the "values" field
	extractValuesField(resMap, result)

	// Extract values from the "change.after" field
	extractChangeAfterField(resMap, result)

	return result
}

// extractValuesField extracts attributes from the "values" field of a resource.
func extractValuesField(resMap map[string]interface{}, result map[string]interface{}) {
	values, ok := resMap["values"].(map[string]interface{})
	if !ok {
		return
	}

	for k, v := range values {
		result[k] = v
	}
}

// extractChangeAfterField extracts attributes from the "change.after" field of a resource.
func extractChangeAfterField(resMap map[string]interface{}, result map[string]interface{}) {
	change, ok := resMap["change"].(map[string]interface{})
	if !ok {
		return
	}

	after, ok := change["after"].(map[string]interface{})
	if !ok {
		return
	}

	for k, v := range after {
		result[k] = v
	}
}

// getUnknownAttributes returns the top-level attributes that Terraform reports as unknown until
// apply (change.after_unknown[attr] == true). Only a boolean true counts; nested unknown
// structures are not attribute-level unknowns.
func getUnknownAttributes(resource interface{}) map[string]bool {
	result := make(map[string]bool)

	resMap, ok := resource.(map[string]interface{})
	if !ok {
		return result
	}

	change, ok := resMap["change"].(map[string]interface{})
	if !ok {
		return result
	}

	unknown, ok := change["after_unknown"].(map[string]interface{})
	if !ok {
		return result
	}

	for k, v := range unknown {
		if isUnknown, ok := v.(bool); ok && isUnknown {
			result[k] = true
		}
	}

	return result
}
