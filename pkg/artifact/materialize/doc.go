// Package materialize extracts verified artifact content into a local directory.
//
// Extraction always happens in a private staging directory next to the destination, written through
// an os.Root so no entry can land outside it. Every path is validated against the Record
// inventory: traversal, entries absent from the inventory, escaping or chained symlinks, special
// members, size or digest mismatches, and oversize content are explicit errors. A newly pulled tree
// is never merged over old files: replacing an existing destination deletes the old children
// inside the destination root only and then moves the staged tree in. Ordinary permission bits
// (including executable bits) are restored exactly; owner, group, and ACLs never are.
package materialize
