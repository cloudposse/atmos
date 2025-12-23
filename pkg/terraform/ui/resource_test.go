package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResourceTracker_NewResourceTracker(t *testing.T) {
	rt := NewResourceTracker()
	assert.NotNil(t, rt)
	assert.Equal(t, PhaseInitializing, rt.GetPhase())
	assert.Equal(t, 0, rt.GetTotalCount())
	assert.Equal(t, 0, rt.GetActiveCount())
	assert.Equal(t, 0, rt.GetCompletedCount())
	assert.False(t, rt.HasErrors())
}

func TestResourceTracker_HandlePlannedChange(t *testing.T) {
	rt := NewResourceTracker()

	msg := &PlannedChangeMessage{
		Change: PlannedChange{
			Resource: ResourceAddr{
				Addr:         "aws_instance.example",
				Module:       "",
				ResourceType: "aws_instance",
				ResourceName: "example",
			},
			Action: "create",
		},
	}

	rt.HandleMessage(msg)

	assert.Equal(t, PhasePlanning, rt.GetPhase())
	assert.Equal(t, 1, rt.GetTotalCount())

	resources := rt.GetResources()
	assert.Len(t, resources, 1)
	assert.Equal(t, "aws_instance.example", resources[0].Address)
	assert.Equal(t, "create", resources[0].Action)
	assert.Equal(t, ResourceStatePending, resources[0].State)
}

func TestResourceTracker_HandleApplyStart(t *testing.T) {
	rt := NewResourceTracker()

	// First add a planned change.
	planned := &PlannedChangeMessage{
		Change: PlannedChange{
			Resource: ResourceAddr{
				Addr:         "aws_instance.example",
				ResourceType: "aws_instance",
				ResourceName: "example",
			},
			Action: "create",
		},
	}
	rt.HandleMessage(planned)

	// Now start applying.
	start := &ApplyStartMessage{
		Hook: ApplyHook{
			Resource: ResourceAddr{
				Addr:         "aws_instance.example",
				ResourceType: "aws_instance",
				ResourceName: "example",
			},
			Action:  "create",
			IDKey:   "id",
			IDValue: "",
		},
	}
	rt.HandleMessage(start)

	assert.Equal(t, PhaseApplying, rt.GetPhase())
	assert.Equal(t, 1, rt.GetActiveCount())

	resources := rt.GetResources()
	assert.Len(t, resources, 1)
	assert.Equal(t, ResourceStateInProgress, resources[0].State)
}

func TestResourceTracker_HandleApplyComplete(t *testing.T) {
	rt := NewResourceTracker()

	// Add planned change and start.
	rt.HandleMessage(&PlannedChangeMessage{
		Change: PlannedChange{
			Resource: ResourceAddr{Addr: "aws_instance.example"},
			Action:   "create",
		},
	})
	rt.HandleMessage(&ApplyStartMessage{
		Hook: ApplyHook{
			Resource: ResourceAddr{Addr: "aws_instance.example"},
			Action:   "create",
		},
	})

	// Complete.
	complete := &ApplyCompleteMessage{
		Hook: ApplyHook{
			Resource:    ResourceAddr{Addr: "aws_instance.example"},
			Action:      "create",
			IDKey:       "id",
			IDValue:     "i-1234567890",
			ElapsedSecs: 10,
		},
	}
	rt.HandleMessage(complete)

	assert.Equal(t, 0, rt.GetActiveCount())
	assert.Equal(t, 1, rt.GetCompletedCount())

	resources := rt.GetResources()
	assert.Len(t, resources, 1)
	assert.Equal(t, ResourceStateComplete, resources[0].State)
	assert.Equal(t, 10, resources[0].ElapsedSecs)
	assert.Equal(t, "i-1234567890", resources[0].IDValue)
}

func TestResourceTracker_HandleApplyErrored(t *testing.T) {
	rt := NewResourceTracker()

	rt.HandleMessage(&PlannedChangeMessage{
		Change: PlannedChange{
			Resource: ResourceAddr{Addr: "aws_instance.example"},
			Action:   "create",
		},
	})
	rt.HandleMessage(&ApplyStartMessage{
		Hook: ApplyHook{
			Resource: ResourceAddr{Addr: "aws_instance.example"},
			Action:   "create",
		},
	})

	errored := &ApplyErroredMessage{
		BaseMessage: BaseMessage{Message: "Error: creating EC2 Instance"},
		Hook: ApplyHook{
			Resource:    ResourceAddr{Addr: "aws_instance.example"},
			Action:      "create",
			ElapsedSecs: 5,
		},
	}
	rt.HandleMessage(errored)

	assert.Equal(t, PhaseError, rt.GetPhase())
	assert.True(t, rt.HasErrors())
	assert.Equal(t, 1, rt.GetErrorCount())

	resources := rt.GetResources()
	assert.Equal(t, ResourceStateError, resources[0].State)
	assert.Contains(t, resources[0].Error, "Error: creating EC2 Instance")
}

func TestResourceTracker_HandleRefresh(t *testing.T) {
	rt := NewResourceTracker()

	start := &RefreshStartMessage{
		Hook: RefreshHook{
			Resource: ResourceAddr{
				Addr:         "aws_instance.example",
				ResourceType: "aws_instance",
				ResourceName: "example",
			},
			IDKey:   "id",
			IDValue: "i-123",
		},
	}
	rt.HandleMessage(start)

	assert.Equal(t, PhaseRefreshing, rt.GetPhase())
	assert.Equal(t, 1, rt.GetTotalCount())

	resources := rt.GetResources()
	assert.Equal(t, ResourceStateRefreshing, resources[0].State)

	complete := &RefreshCompleteMessage{
		Hook: RefreshHook{
			Resource: ResourceAddr{Addr: "aws_instance.example"},
		},
	}
	rt.HandleMessage(complete)

	resources = rt.GetResources()
	assert.Equal(t, ResourceStateComplete, resources[0].State)
}

func TestResourceTracker_HandleDiagnostic(t *testing.T) {
	rt := NewResourceTracker()

	// Warning diagnostic.
	warning := &DiagnosticMessage{
		Diagnostic: Diagnostic{
			Severity: "warning",
			Summary:  "Deprecated attribute",
			Detail:   "This attribute is deprecated",
		},
	}
	rt.HandleMessage(warning)

	diags := rt.GetDiagnostics()
	assert.Len(t, diags, 1)
	assert.Equal(t, "warning", diags[0].Diagnostic.Severity)
	assert.False(t, rt.HasErrors())

	// Error diagnostic.
	errDiag := &DiagnosticMessage{
		Diagnostic: Diagnostic{
			Severity: "error",
			Summary:  "Invalid configuration",
			Detail:   "Expected string, got number",
		},
	}
	rt.HandleMessage(errDiag)

	diags = rt.GetDiagnostics()
	assert.Len(t, diags, 2)
	assert.Equal(t, PhaseError, rt.GetPhase())
	assert.True(t, rt.HasErrors())
}

func TestResourceTracker_HandleChangeSummary(t *testing.T) {
	rt := NewResourceTracker()

	summary := &ChangeSummaryMessage{
		Changes: Changes{
			Add:       2,
			Change:    1,
			Remove:    1,
			Operation: "apply",
		},
	}
	rt.HandleMessage(summary)

	assert.Equal(t, PhaseComplete, rt.GetPhase())

	result := rt.GetChangeSummary()
	assert.NotNil(t, result)
	assert.Equal(t, 2, result.Changes.Add)
	assert.Equal(t, 1, result.Changes.Change)
	assert.Equal(t, 1, result.Changes.Remove)
}

func TestResourceTracker_HandlePlanChangeSummary(t *testing.T) {
	rt := NewResourceTracker()

	// Plan operations also receive change_summary.
	summary := &ChangeSummaryMessage{
		Changes: Changes{
			Add:       1,
			Change:    0,
			Remove:    0,
			Operation: "plan",
		},
	}
	rt.HandleMessage(summary)

	// Plan should also mark as complete.
	assert.Equal(t, PhaseComplete, rt.GetPhase())

	result := rt.GetChangeSummary()
	assert.NotNil(t, result)
	assert.Equal(t, 1, result.Changes.Add)
	assert.Equal(t, 0, result.Changes.Change)
	assert.Equal(t, 0, result.Changes.Remove)
	assert.Equal(t, "plan", result.Changes.Operation)
}

func TestResourceTracker_HandleVersionMessage(t *testing.T) {
	rt := NewResourceTracker()

	version := &VersionMessage{
		Terraform: "1.9.0",
		UI:        "1.2",
	}
	rt.HandleMessage(version)

	// Version shouldn't change state significantly.
	assert.Equal(t, PhaseInitializing, rt.GetPhase())
}

func TestResourceTracker_ResourceOrder(t *testing.T) {
	rt := NewResourceTracker()

	// Add resources in specific order.
	for _, addr := range []string{"aws_vpc.main", "aws_subnet.a", "aws_instance.web"} {
		rt.HandleMessage(&PlannedChangeMessage{
			Change: PlannedChange{
				Resource: ResourceAddr{Addr: addr},
				Action:   "create",
			},
		})
	}

	resources := rt.GetResources()
	assert.Len(t, resources, 3)
	assert.Equal(t, "aws_vpc.main", resources[0].Address)
	assert.Equal(t, "aws_subnet.a", resources[1].Address)
	assert.Equal(t, "aws_instance.web", resources[2].Address)
}

func TestResourceTracker_Concurrency(t *testing.T) {
	rt := NewResourceTracker()

	done := make(chan bool, 100)

	// Simulate concurrent reads and writes.
	for i := 0; i < 50; i++ {
		go func(n int) {
			rt.HandleMessage(&PlannedChangeMessage{
				Change: PlannedChange{
					Resource: ResourceAddr{Addr: "aws_instance.test_" + string(rune('0'+n%10))},
					Action:   "create",
				},
			})
			done <- true
		}(i)

		go func() {
			_ = rt.GetResources()
			_ = rt.GetTotalCount()
			_ = rt.GetActiveCount()
			_ = rt.HasErrors()
			done <- true
		}()
	}

	// Wait for all goroutines.
	for i := 0; i < 100; i++ {
		<-done
	}

	// Verify we have some resources.
	assert.Greater(t, rt.GetTotalCount(), 0)
}

func TestResourceTracker_HandleApplyStart_UnplannedResource(t *testing.T) {
	rt := NewResourceTracker()

	// Apply start for resource NOT in plan (edge case for dynamic resources).
	start := &ApplyStartMessage{
		Hook: ApplyHook{
			Resource: ResourceAddr{
				Addr:         "aws_instance.dynamic",
				Module:       "module.dynamic",
				ResourceType: "aws_instance",
				ResourceName: "dynamic",
			},
			Action:  "create",
			IDKey:   "id",
			IDValue: "",
		},
	}
	rt.HandleMessage(start)

	assert.Equal(t, PhaseApplying, rt.GetPhase())
	assert.Equal(t, 1, rt.GetTotalCount())
	assert.Equal(t, 1, rt.GetActiveCount())

	resources := rt.GetResources()
	require.Len(t, resources, 1)
	assert.Equal(t, "aws_instance.dynamic", resources[0].Address)
	assert.Equal(t, ResourceStateInProgress, resources[0].State)
	assert.Equal(t, "module.dynamic", resources[0].Module)
}

func TestResourceTracker_HandleRefreshStart_ExistingResource(t *testing.T) {
	rt := NewResourceTracker()

	// Add planned change first.
	rt.HandleMessage(&PlannedChangeMessage{
		Change: PlannedChange{
			Resource: ResourceAddr{
				Addr:         "aws_instance.web",
				ResourceType: "aws_instance",
				ResourceName: "web",
			},
			Action: "update",
		},
	})

	// Verify initial state.
	resources := rt.GetResources()
	require.Len(t, resources, 1)
	assert.Equal(t, ResourceStatePending, resources[0].State)

	// Now refresh the same resource.
	rt.HandleMessage(&RefreshStartMessage{
		Hook: RefreshHook{
			Resource: ResourceAddr{
				Addr:         "aws_instance.web",
				ResourceType: "aws_instance",
				ResourceName: "web",
			},
			IDKey:   "id",
			IDValue: "i-existing",
		},
	})

	// Verify state changed to refreshing.
	assert.Equal(t, PhaseRefreshing, rt.GetPhase())
	resources = rt.GetResources()
	require.Len(t, resources, 1)
	assert.Equal(t, ResourceStateRefreshing, resources[0].State)
}

func TestResourceTracker_HandleOutputs(t *testing.T) {
	rt := NewResourceTracker()

	outputs := &OutputsMessage{
		Outputs: map[string]OutputValue{
			"vpc_id":      {Value: "vpc-123abc", Sensitive: false},
			"db_password": {Value: "secret", Sensitive: true},
		},
	}
	rt.HandleMessage(outputs)

	result := rt.GetOutputs()
	require.NotNil(t, result)
	require.Len(t, result.Outputs, 2)
	assert.Equal(t, "vpc-123abc", result.Outputs["vpc_id"].Value)
	assert.False(t, result.Outputs["vpc_id"].Sensitive)
	assert.True(t, result.Outputs["db_password"].Sensitive)
}

func TestResourceTracker_GetCurrentActivity_NoActive(t *testing.T) {
	rt := NewResourceTracker()

	// Add a completed resource.
	rt.HandleMessage(&PlannedChangeMessage{
		Change: PlannedChange{
			Resource: ResourceAddr{Addr: "aws_vpc.main"},
			Action:   "create",
		},
	})

	// No active resources yet (only pending).
	activity := rt.GetCurrentActivity()
	assert.Nil(t, activity)
}

func TestResourceTracker_GetCurrentActivity_WithActive(t *testing.T) {
	rt := NewResourceTracker()

	// Add and start a resource.
	rt.HandleMessage(&PlannedChangeMessage{
		Change: PlannedChange{
			Resource: ResourceAddr{Addr: "aws_vpc.main"},
			Action:   "create",
		},
	})
	rt.HandleMessage(&ApplyStartMessage{
		Hook: ApplyHook{
			Resource: ResourceAddr{Addr: "aws_vpc.main"},
			Action:   "create",
		},
	})

	activity := rt.GetCurrentActivity()
	require.NotNil(t, activity)
	assert.Equal(t, "aws_vpc.main", activity.Address)
	assert.Equal(t, ResourceStateInProgress, activity.State)
}

func TestResourceTracker_HandleApplyProgress(t *testing.T) {
	rt := NewResourceTracker()

	// Set up resource in progress.
	rt.HandleMessage(&PlannedChangeMessage{
		Change: PlannedChange{
			Resource: ResourceAddr{Addr: "aws_instance.web"},
			Action:   "create",
		},
	})
	rt.HandleMessage(&ApplyStartMessage{
		Hook: ApplyHook{
			Resource: ResourceAddr{Addr: "aws_instance.web"},
			Action:   "create",
		},
	})

	// Send progress update.
	rt.HandleMessage(&ApplyProgressMessage{
		Hook: ApplyHook{
			Resource:    ResourceAddr{Addr: "aws_instance.web"},
			Action:      "create",
			ElapsedSecs: 30,
		},
	})

	resources := rt.GetResources()
	require.Len(t, resources, 1)
	assert.Equal(t, 30, resources[0].ElapsedSecs)
	assert.Equal(t, ResourceStateInProgress, resources[0].State)
}

func TestResourceTracker_ChangeSummaryWithErrorDoesNotOverridePhase(t *testing.T) {
	rt := NewResourceTracker()

	// First, error diagnostic.
	rt.HandleMessage(&DiagnosticMessage{
		Diagnostic: Diagnostic{
			Severity: "error",
			Summary:  "Configuration error",
		},
	})
	assert.Equal(t, PhaseError, rt.GetPhase())

	// Change summary after error should not change phase back to complete.
	rt.HandleMessage(&ChangeSummaryMessage{
		Changes: Changes{
			Add:       0,
			Change:    0,
			Remove:    0,
			Operation: "apply",
		},
	})
	assert.Equal(t, PhaseError, rt.GetPhase())
}
