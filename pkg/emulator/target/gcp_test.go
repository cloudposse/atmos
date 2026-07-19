package target

import (
	"testing"

	"github.com/stretchr/testify/assert"

	emu "github.com/cloudposse/atmos/pkg/emulator"
)

func TestGCPProfile_Branches(t *testing.T) {
	t.Run("bound port and project set all hosts", func(t *testing.T) {
		ep := &emu.Endpoint{Target: emu.TargetGCP, Host: "localhost", Ports: map[int]int{4588: 30001}, Project: "demo"}
		p := GCPProfile(ep)

		assert.Equal(t, "true", p.Env["CLOUDSDK_AUTH_DISABLE_CREDENTIALS"])

		// GCS wants a URL; the other emulator hosts want a bare host:port authority.
		assert.Equal(t, "http://127.0.0.1:30001", p.Env["STORAGE_EMULATOR_HOST"])
		assert.Equal(t, "127.0.0.1:30001", p.Env["PUBSUB_EMULATOR_HOST"])
		assert.Equal(t, "127.0.0.1:30001", p.Env["FIRESTORE_EMULATOR_HOST"])
		assert.Equal(t, "127.0.0.1:30001", p.Env["BIGTABLE_EMULATOR_HOST"])
		assert.Equal(t, "127.0.0.1:30001", p.Env["DATASTORE_EMULATOR_HOST"])

		// Project appears in both the gcloud core project and the SDK env var.
		assert.Equal(t, "demo", p.Env["CLOUDSDK_CORE_PROJECT"])
		assert.Equal(t, "demo", p.Env["GOOGLE_CLOUD_PROJECT"])

		assert.Equal(t, "demo", p.Provider["project"])
		assert.Equal(t, "test", p.Provider["access_token"])
		assert.Equal(t, false, p.Provider["user_project_override"])
		assert.Equal(t, "http://127.0.0.1:30001/storage/v1/", p.Provider["storage_custom_endpoint"])
		assert.Equal(t, "http://127.0.0.1:30001/v1/", p.Provider["secret_manager_custom_endpoint"])
		assert.Equal(t, "http://127.0.0.1:30001/", p.Provider["iam_custom_endpoint"])
	})

	t.Run("no bound port and no project omits hosts and defaults project", func(t *testing.T) {
		ep := &emu.Endpoint{Target: emu.TargetGCP, Host: "localhost", Ports: map[int]int{}}
		p := GCPProfile(ep)

		// The credential-disable flag is unconditional.
		assert.Equal(t, "true", p.Env["CLOUDSDK_AUTH_DISABLE_CREDENTIALS"])

		// Without a live authority, none of the *_EMULATOR_HOST keys are set.
		assert.NotContains(t, p.Env, "STORAGE_EMULATOR_HOST")
		assert.NotContains(t, p.Env, "PUBSUB_EMULATOR_HOST")
		assert.NotContains(t, p.Env, "FIRESTORE_EMULATOR_HOST")
		assert.NotContains(t, p.Env, "BIGTABLE_EMULATOR_HOST")
		assert.NotContains(t, p.Env, "DATASTORE_EMULATOR_HOST")

		// Without a project, the Floci-local default is used.
		assert.Equal(t, gcpDefaultProject, p.Env["CLOUDSDK_CORE_PROJECT"])
		assert.Equal(t, gcpDefaultProject, p.Env["GOOGLE_CLOUD_PROJECT"])
		assert.Equal(t, gcpDefaultProject, p.Provider["project"])
		assert.NotContains(t, p.Provider, "storage_custom_endpoint")
	})

	t.Run("bound port without project sets hosts and default project", func(t *testing.T) {
		ep := &emu.Endpoint{Target: emu.TargetGCP, Host: "localhost", Ports: map[int]int{4588: 40002}}
		p := GCPProfile(ep)

		assert.Equal(t, "http://127.0.0.1:40002", p.Env["STORAGE_EMULATOR_HOST"])
		assert.Equal(t, "127.0.0.1:40002", p.Env["PUBSUB_EMULATOR_HOST"])
		assert.Equal(t, gcpDefaultProject, p.Env["GOOGLE_CLOUD_PROJECT"])
		assert.Equal(t, gcpDefaultProject, p.Env["CLOUDSDK_CORE_PROJECT"])
		assert.Equal(t, gcpDefaultProject, p.Provider["project"])
		assert.Equal(t, "http://127.0.0.1:40002/storage/v1/", p.Provider["storage_custom_endpoint"])
	})
}
