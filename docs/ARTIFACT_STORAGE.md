# Artifact storage abstraction

Control Plane no longer reads and writes release files directly.

The ArtifactStorage interface owns:
- atomic verified writes;
- object open/read metadata;
- delete;
- cleanup of stale temporary uploads.

The default backend is Local, rooted at VPNX3_ARTIFACT_DIR. Path traversal checks, temporary files, fsync, atomic rename and SHA-256 verification are isolated inside that backend.

Build job transitions to succeeded only after PutVerified has persisted the object. If the following database transaction fails, Control Plane deletes the object through the same backend.

Admin/public download and retention cleanup use the interface as well. A future S3-compatible implementation can replace Local without changing release/build HTTP contracts or database storage_key semantics.
