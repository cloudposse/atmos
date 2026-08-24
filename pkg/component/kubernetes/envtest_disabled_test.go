//go:build !envtest

package kubernetes

// envtestSetup is a no-op when the `envtest` build tag is absent, so the default
// (fast, fake-client) test tier neither provisions binaries nor starts a control
// plane. The real implementation lives in envtest_test.go (//go:build envtest),
// which boots a kube-apiserver+etcd for the end-to-end test tier.
func envtestSetup() func() { return func() {} }
