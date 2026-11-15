# A plain variable, referenced as [[DOCKER_ROOT]] in stack/deployment config.
resource "komodo_variable" "docker_root" {
  name        = "DOCKER_ROOT"
  value       = "/opt/docker"
  description = "Base directory for stack data"
}

# A secret variable: hidden in Komodo's updates and logs.
resource "komodo_variable" "db_password" {
  name      = "POSTGRES_PASSWORD"
  value     = var.postgres_password
  is_secret = true
}

resource "komodo_stack" "app" {
  name      = "app"
  server_id = "Local"
  repo      = "myorg/app"

  environment = <<-EOT
    DATA_DIR=[[DOCKER_ROOT]]/app
    POSTGRES_PASSWORD=[[POSTGRES_PASSWORD]]
  EOT

  # Variables are resolved by name at deploy time, so make the dependency explicit.
  depends_on = [komodo_variable.docker_root, komodo_variable.db_password]
}
