# Look up an existing registry account (e.g. created via the UI) by ID.
data "komodo_docker_registry_account" "ghcr" {
  id = "65a1b2c3d4e5f6a7b8c9d0e1"
}
