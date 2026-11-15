resource "komodo_api_key" "ci_bot" {
  user_id = "65a1b2c3d4e5f6a7b8c9d0e1" # komodo_user.<name>.id
  name    = "ci-bot-default"
}

# A key that expires. `expires` is a unix timestamp in milliseconds;
# changing it replaces the key.
resource "komodo_api_key" "contractor" {
  user_id = "65a1b2c3d4e5f6a7b8c9d0e1"
  name    = "contractor-2027"
  expires = 1798761600000 # 2027-01-01T00:00:00Z
}

# `secret` is only returned at create time — capture it now (it cannot be
# read back later).
output "ci_bot_api_secret" {
  value     = komodo_api_key.ci_bot.secret
  sensitive = true
}
