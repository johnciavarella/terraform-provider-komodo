resource "komodo_onboarding_key" "fleet" {
  name           = "fleet-bootstrap"
  enabled        = true
  expires        = 0 # 0 = no expiry; otherwise unix-millis
  privileged     = false
  create_builder = true # also provision a Builder for each onboarded server
}

# `private_key` is only returned at create time — capture it now.
output "fleet_private_key" {
  value     = komodo_onboarding_key.fleet.private_key
  sensitive = true
}
