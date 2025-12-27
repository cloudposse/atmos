package workdir

import "time"

// CreateSampleWorkdirInfo creates a sample WorkdirInfo for testing.
func CreateSampleWorkdirInfo(component, stack string) *WorkdirInfo {
	return &WorkdirInfo{
		Name:        stack + "-" + component,
		Component:   component,
		Stack:       stack,
		Source:      "components/terraform/" + component,
		Path:        ".workdir/terraform/" + stack + "-" + component,
		ContentHash: "abc123",
		CreatedAt:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

// CreateSampleWorkdirList creates a sample list of workdirs for testing.
func CreateSampleWorkdirList() []WorkdirInfo {
	return []WorkdirInfo{
		*CreateSampleWorkdirInfo("vpc", "dev"),
		*CreateSampleWorkdirInfo("vpc", "prod"),
		*CreateSampleWorkdirInfo("s3", "dev"),
	}
}
