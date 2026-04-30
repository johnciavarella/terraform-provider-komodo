resource "komodo_docker_registry_account" "ghcr" {
  domain   = "ghcr.io"
  username = "my-github-user"
  token    = "ghp_…" # use var.* / a secret store in real configs (sensitive)
}
