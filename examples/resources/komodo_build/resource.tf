resource "komodo_build" "api" {
  name                   = "api"
  builder_id             = "default-builder" # or a komodo_builder.<name>.id
  image_name             = "myorg/api"
  auto_increment_version = true
  include_latest_tag     = true

  git_provider = "github.com"
  git_account  = "my-github-user"
  repo         = "myorg/api"
  branch       = "main"

  build_path        = "."
  dockerfile_path   = "Dockerfile"
  build_args        = "GO_VERSION=1.22"
  pre_build_command = "./scripts/codegen.sh"

  # v2: list of registry destinations.
  image_registry = [
    {
      domain       = "ghcr.io"
      account      = "my-github-user"
      organization = "myorg"
    },
  ]
}
