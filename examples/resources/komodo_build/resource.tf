resource "komodo_build" "api" {
  name                   = "api"
  description            = "API service image"
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

  # Passed with docker secret mounts, so not visible in the final image.
  secret_args = "NPM_TOKEN=${var.npm_token}"
  extra_args  = ["--pull", "--no-cache"]

  # v2: list of registry destinations.
  image_registry = [
    {
      domain       = "ghcr.io"
      account      = "my-github-user"
      organization = "myorg"
    },
  ]
}

# Pin the version yourself instead of letting Komodo bump it on every build.
resource "komodo_build" "base_image" {
  name                   = "base-image"
  repo                   = "myorg/base-image"
  version                = "3.2.0"
  auto_increment_version = false
}
