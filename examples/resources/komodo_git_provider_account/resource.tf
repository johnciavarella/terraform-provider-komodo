resource "komodo_git_provider_account" "github" {
  domain   = "github.com"
  username = "my-github-user"
  token    = "ghp_…" # use var.* / a secret store in real configs (sensitive)
  https    = true
}
