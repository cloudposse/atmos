package driver

import (
	"fmt"

	emu "github.com/cloudposse/atmos/pkg/emulator"
	"github.com/cloudposse/atmos/pkg/emulator/target"
)

// Gitea is a lightweight, self-hosted Git service that serves Git over HTTP from
// a single container — the `git` emulator target. Unlike a bare `git daemon`
// (the `git://` protocol), Gitea speaks HTTP, which is what GitOps controllers
// (Flux, Argo CD) require to watch a repository. That makes one emulator serve
// both halves of the loop: Atmos pushes rendered manifests to it, and a
// controller running in the k3s emulator reconciles them back out.
//
// A fresh Gitea boots installed-but-empty (INSTALL_LOCK skips the web wizard);
// the manager's git bootstrap then creates the admin user and the deployment
// repository, mirroring the Vault/OpenBao bootstrap.
const (
	giteaImage = "gitea/gitea:1.22"
	// The giteaPort is Gitea's default HTTP listener (the smart-HTTP Git endpoint and API).
	giteaPort = 3000
	// The giteaDataDir is Gitea's persistent state root inside the container (its
	// declared volume: the SQLite database, repositories, and config). Persisting
	// it keeps repos and the admin user across `down`/`up`.
	giteaDataDir = "/data"
)

// giteaEnv configures a headless install so Gitea comes up ready to serve without
// the interactive web setup wizard: SQLite storage (no external DB), the install
// lock set (skip the wizard), and the public URL/port. The manager's git
// bootstrap handles the admin user and repository on top of this.
func giteaEnv() map[string]string {
	return map[string]string{
		"GITEA__database__DB_TYPE":      "sqlite3",
		"GITEA__security__INSTALL_LOCK": "true",
		"GITEA__server__HTTP_PORT":      fmt.Sprintf("%d", giteaPort),
		"GITEA__server__ROOT_URL":       fmt.Sprintf("http://localhost:%d/", giteaPort),
		"GITEA__server__DOMAIN":         "localhost",
		// Keep the demo self-contained: no email, no captcha, local accounts only.
		"GITEA__service__DISABLE_REGISTRATION": "true",
	}
}

func init() {
	// Gitea exposes a readiness endpoint at /api/healthz that returns 200 once the
	// server is serving. busybox wget (present in the Alpine-based image) gives a
	// dependency-free probe, matching the registry driver's pattern.
	healthCheck := shellHealthCheck(fmt.Sprintf("wget -q -O /dev/null http://localhost:%d/api/healthz || exit 1", giteaPort))
	emu.RegisterDriver(&builtinDriver{
		name:        "gitea",
		target:      emu.TargetGit,
		image:       giteaImage,
		ports:       []int{giteaPort},
		env:         giteaEnv(),
		dataDir:     giteaDataDir,
		healthCheck: healthCheck,
		restart:     defaultEmulatorRestart,
		profile:     target.GitProfile,
	})
}
