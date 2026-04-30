resource "komodo_server" "web_01" {
  name             = "web-01"
  address          = "wss://web-01.example.com:8120"
  region           = "us-east-1"
  enabled          = true
  auto_rotate_keys = true # let Komodo manage the v2 PKI keypair

  send_cpu_alerts = true
  cpu_warning     = 80
  cpu_critical    = 95

  links = ["https://wiki.example.com/runbooks/web-01"]
  tags  = ["production"]
}
