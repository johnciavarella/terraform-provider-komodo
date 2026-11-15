resource "komodo_stack" "web" {
  name          = "web-app"
  description   = "Public web application"
  server_id     = "Local"
  files_on_host = true

  environment = "API_HOST=https://api.example.com"

  # v2: list of objects, not list of strings.
  additional_env_files = [
    { path = ".env.shared" },                # track defaults to true
    { path = ".env.secret", track = false }, # passed via --env-file only
  ]

  # Extra files Komodo tracks. A change counts in DeployStackIfChanged.
  config_files = [
    { path = "nginx.conf", services = ["web"], requires = "Restart" },
    { path = "app.toml", requires = "Redeploy" },
  ]

  pre_deploy_path    = "./"
  pre_deploy_command = "./scripts/pre-deploy.sh"

  poll_for_updates          = true
  auto_update               = true
  auto_update_skip_services = ["db"] # never auto-update the database

  tags = ["production"]
}
