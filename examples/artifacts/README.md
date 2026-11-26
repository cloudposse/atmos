---
title: Artifacts
tags: [Automation]
cast:
  file: /casts/examples/artifacts/publish-and-restore.cast
  title: Publish and restore a local artifact
related_docs:
  - label: Artifact repositories
    url: /artifacts
  - label: Artifact commands
    url: /cli/commands/artifacts/usage
---

# Artifacts

Publish a directory as a [versioned artifact](https://atmos.tools/artifacts), list it in a local repository, and restore its files. This example runs entirely on your filesystem.

From the repository root, run these commands with a version of Atmos that includes `atmos artifacts`:

```shell
cd examples/artifacts
atmos artifacts publish ./bundle --repository=local --name=builds/app --version=1.0.0
atmos artifacts list --repository=local
atmos artifacts pull builds/app:1.0.0 --repository=local --output=./restored
diff -r bundle restored
```

The list includes `builds/app` at version `1.0.0`. An empty diff confirms the restored files match the original bundle.
