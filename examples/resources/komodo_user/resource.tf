# Service user — authenticates with an API key.
resource "komodo_user" "ci_bot" {
  username  = "ci-bot"
  user_type = "service"
}

# Local user — authenticates with username + password.
# create_*_permissions are honored only on local users.
resource "komodo_user" "operator" {
  username                  = "ops-operator"
  user_type                 = "local"
  password                  = "change-me" # use var.* / a secret store in real configs
  create_server_permissions = true
}
