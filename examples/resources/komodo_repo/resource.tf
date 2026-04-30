resource "komodo_repo" "tooling" {
  name         = "tooling"
  server_id    = "Local"
  git_provider = "github.com"
  git_account  = "my-github-user"
  repo         = "myorg/tooling"
  branch       = "main"

  # v2: SystemCommand is split into _path / _command pairs.
  on_clone_path    = "./"
  on_clone_command = "make setup"
  on_pull_path     = "./"
  on_pull_command  = "make sync"

  webhook_enabled = true
  links           = ["https://github.com/myorg/tooling"]
}
