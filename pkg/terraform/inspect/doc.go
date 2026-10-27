// Package inspect loads Terraform and OpenTofu root modules with terraform-config-inspect and
// classifies the diagnostics that come from the library's static parser rather than from the
// configuration itself.
//
// [LoadModule] wraps [tfconfig.LoadModule], so file ordering, override files, the legacy HCL
// fallback and provider-requirement initialization are unchanged. When the library reports
// "Invalid provider reference", LoadModule parses the referenced file again with HashiCorp HCL
// (native syntax for `.tf`, JSON for `.tf.json`), reruns [tfconfig.LoadModuleFromFile] on it to
// get the diagnostic's full source range, and checks the `provider` expression at that range. Only
// an indexed provider alias such as `aws.by_account[each.key]` (OpenTofu 1.9+ provider iteration)
// is set aside. Every other diagnostic, including other invalid provider expressions such as a
// conditional or a function call, is returned to the caller unchanged.
//
// Policy stays with the caller: this package does not decide which of the remaining diagnostics
// are fatal.
package inspect
