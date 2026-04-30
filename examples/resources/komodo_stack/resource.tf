resource "komodo_stack" "web" {
  name          = "web-app"
  server_id     = "Local"
  files_on_host = true

  environment = "API_HOST=https://api.example.com"

  # v2: list of objects, not list of strings.
  additional_env_files = [
    { path = ".env.shared" },                # track defaults to true
    { path = ".env.secret", track = false }, # passed via --env-file only
  ]

  pre_deploy_path    = "./"
  pre_deploy_command = "./scripts/pre-deploy.sh"

  poll_for_updates = true
  tags             = ["production"]
}
