package kube

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/cloudposse/atmos/pkg/auth/realm"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ClusterInfo is the cloud-agnostic cluster data needed to write a kubeconfig
// entry. Each cloud package (aws, azure) builds one of these from its own
// describe-cluster call before handing it to KubeconfigManager.
type ClusterInfo struct {
	// Name is the cluster's short name.
	Name string

	// Endpoint is the cluster's API server URL.
	Endpoint string

	// CertificateAuthorityData is the base64-encoded CA certificate, as
	// returned by the cloud API. Decoded the same way for every cloud.
	CertificateAuthorityData string

	// ID uniquely identifies the cluster and is used as the kubeconfig
	// cluster map key and default context name: the ARN for EKS, the ARM
	// resource ID for AKS, or the canonical GKE resource name. With a
	// non-empty Realm, the default context name is <realm>/<ID> (see
	// ContextName).
	ID string

	// Region disambiguates the generated exec-plugin username when the same
	// cluster name exists in more than one place: the AWS region for EKS,
	// the resource group for AKS, or the project and location for GKE.
	Region string

	// UserPrefix distinguishes the exec-plugin username by cloud, e.g. "eks"
	// or "aks", reproducing the existing "atmos-eks-<name>-<region>" scheme.
	UserPrefix string

	// AccountID further disambiguates the exec-plugin username for clouds
	// where Name+Region is not guaranteed unique: Azure resource groups are
	// scoped per-subscription, so the same cluster name and resource group
	// can exist in two subscriptions, and EKS clusters of the same name and
	// region can exist in two AWS accounts. Leave empty to omit it.
	AccountID string

	// Identity is the Atmos identity the exec plugin authenticates as. It is
	// part of the exec-plugin username so two contexts for the same cluster
	// that use different identities never share (and overwrite) one user.
	// Leave empty to omit it.
	Identity string

	// Realm is the Atmos auth realm the entry was written from. A non-empty
	// realm is part of the exec-plugin username, so repositories with
	// different realms never share (and overwrite) one user in a shared
	// kubeconfig, and it is passed to the exec plugin as ATMOS_AUTH_REALM so
	// the token is resolved in that realm. It also prefixes the default
	// context name (see ContextName). Leave empty to omit all three. Set it
	// only for a realm the user configured explicitly (see NameRealm).
	Realm string

	// LegacyRealms are realms that unreleased builds prefixed this entry's
	// user and default context names with although the user never configured
	// them: the realm Atmos derives from the atmos.yaml path. Names under
	// these realms count as this entry's own (see OwnedUserNames and
	// ContextNames), so the next write or cleanup removes them. Leave empty
	// when there are none.
	LegacyRealms []string

	// LegacyUserNames are exec-plugin user names that older versions wrote
	// for this entry in formats UserName no longer produces, such as names
	// without the account ID or identity. They count as this entry's own
	// users (see OwnedUserNames) when an older unprefixed context is
	// migrated to the realm-prefixed name. Leave empty when there are none.
	LegacyUserNames []string

	// ExecArgs are the fully-built kubectl exec-credential-plugin arguments,
	// e.g. [aws, eks, token, --cluster-name, X, --region, Y] or
	// [azure, aks, token, --cluster-name, X, --resource-group, Y]. Built by
	// the caller so this package stays cloud-agnostic.
	ExecArgs []string

	// ExecEnv are additional environment variables for the exec plugin, e.g.
	// ATMOS_IDENTITY.
	ExecEnv []clientcmdapi.ExecEnvVar
}

// errWrapFormat wraps a sentinel error around the underlying cause.
const errWrapFormat = "%w: %w"

// userNameHashLen is the number of hex characters of the collision-guard
// hash appended to realm- or identity-scoped user names.
const userNameHashLen = 8

// UserName returns the exec-plugin username for a cluster entry:
// atmos-[<realm>-]<prefix>-<name>-<region>[-<account>][-<identity>].
// The realm comes right after "atmos-" so an empty realm keeps the names
// earlier versions wrote. When the name includes a realm or an identity, a
// "-<hash>" suffix is appended: the first 8 hex characters of a SHA-256 over
// every component joined with a NUL byte. The components themselves are joined
// with "-", which realms, cluster names and identities may contain, so without
// the hash different tuples could produce the same name (realm "r" with
// cluster "b-eks-c" versus realm "r-eks-b" with cluster "c") and one entry
// would overwrite the other's user. Names without a realm or identity are
// unchanged from earlier versions.
func UserName(info *ClusterInfo) string {
	defer perf.Track(nil, "kube.UserName")()

	name := UnhashedUserName(info)
	if info.Realm == "" && info.Identity == "" {
		return name
	}
	sum := sha256.Sum256([]byte(strings.Join([]string{
		info.Realm, info.UserPrefix, info.Name, info.Region, info.AccountID, info.Identity,
	}, "\x00")))
	return name + "-" + hex.EncodeToString(sum[:])[:userNameHashLen]
}

// UnhashedUserName returns the exec-plugin username without the hash suffix:
// the form earlier builds wrote for realm- or identity-scoped entries. It is
// only used to recognize those names as owned for cleanup and migration; new
// entries are always written under UserName.
func UnhashedUserName(info *ClusterInfo) string {
	defer perf.Track(nil, "kube.UnhashedUserName")()

	parts := []string{"atmos"}
	if info.Realm != "" {
		parts = append(parts, info.Realm)
	}
	parts = append(parts, info.UserPrefix, info.Name, info.Region)
	if info.AccountID != "" {
		parts = append(parts, info.AccountID)
	}
	if info.Identity != "" {
		parts = append(parts, info.Identity)
	}
	return strings.Join(parts, "-")
}

// UserNameForms returns UserName followed by UnhashedUserName when the two
// differ, so cleanup recognizes both the current and the earlier form.
func UserNameForms(info *ClusterInfo) []string {
	defer perf.Track(nil, "kube.UserNameForms")()

	current, unhashed := UserName(info), UnhashedUserName(info)
	if current == unhashed {
		return []string{current}
	}
	return []string{current, unhashed}
}

// execEnv returns the exec-plugin environment for info: its ExecEnv plus
// ATMOS_AUTH_REALM when the entry has a realm that ExecEnv does not set.
func execEnv(info *ClusterInfo) []clientcmdapi.ExecEnvVar {
	if info.Realm == "" {
		return info.ExecEnv
	}
	for _, env := range info.ExecEnv {
		if env.Name == realm.EnvVarName {
			return info.ExecEnv
		}
	}
	env := make([]clientcmdapi.ExecEnvVar, 0, len(info.ExecEnv)+1)
	env = append(env, info.ExecEnv...)
	return append(env, clientcmdapi.ExecEnvVar{Name: realm.EnvVarName, Value: info.Realm})
}
