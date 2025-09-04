package exec

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

// testCaseAnsibleSetting represents a test case for Ansible setting extraction.
type testCaseAnsibleSetting struct {
	name     string
	settings *schema.AtmosSectionMapType
	want     string
	wantErr  bool
}

// getAnsibleSettingsTestCases returns common test cases for Ansible settings.
func getAnsibleSettingsTestCases(settingKey, validValue string) []testCaseAnsibleSetting {
	return []testCaseAnsibleSetting{
		{
			name:     "nil settings",
			settings: nil,
			want:     "",
			wantErr:  false,
		},
		{
			name: "no ansible section",
			settings: &schema.AtmosSectionMapType{
				"other": map[string]any{},
			},
			want:    "",
			wantErr: false,
		},
		{
			name: fmt.Sprintf("ansible section with %s", settingKey),
			settings: &schema.AtmosSectionMapType{
				"ansible": map[string]any{
					settingKey: validValue,
				},
			},
			want:    validValue,
			wantErr: false,
		},
		{
			name: fmt.Sprintf("ansible section without %s", settingKey),
			settings: &schema.AtmosSectionMapType{
				"ansible": map[string]any{
					"other": "value",
				},
			},
			want:    "",
			wantErr: false,
		},
		{
			name: "invalid ansible section",
			settings: &schema.AtmosSectionMapType{
				"ansible": "invalid",
			},
			want:    "",
			wantErr: false,
		},
		{
			name: fmt.Sprintf("invalid %s type", settingKey),
			settings: &schema.AtmosSectionMapType{
				"ansible": map[string]any{
					settingKey: 123,
				},
			},
			want:    "",
			wantErr: false,
		},
	}
}

// runAnsibleSettingTest runs the test for a given Ansible setting function.
func runAnsibleSettingTest(t *testing.T, testFunc func(*schema.AtmosSectionMapType) (string, error), testCases []testCaseAnsibleSetting) {
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := testFunc(tt.settings)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetAnsiblePlaybookFromSettings(t *testing.T) {
	testCases := getAnsibleSettingsTestCases("playbook", "site.yml")
	runAnsibleSettingTest(t, GetAnsiblePlaybookFromSettings, testCases)
}

func TestGetAnsibleInventoryFromSettings(t *testing.T) {
	testCases := getAnsibleSettingsTestCases("inventory", "hosts.yml")
	runAnsibleSettingTest(t, GetAnsibleInventoryFromSettings, testCases)
}
