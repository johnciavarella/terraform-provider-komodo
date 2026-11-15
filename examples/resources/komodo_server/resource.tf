resource "komodo_server" "web_01" {
  name             = "web-01"
  description      = "Front-end web host"
  address          = "wss://web-01.example.com:8120"
  region           = "us-east-1"
  enabled          = true
  auto_rotate_keys = true # let Komodo manage the v2 PKI keypair

  send_cpu_alerts              = true
  send_version_mismatch_alerts = false
  cpu_warning                  = 80
  cpu_critical                 = 95

  # Suppress alerts about this server during planned work.
  maintenance_windows = [
    {
      name             = "nightly backup"
      hour             = 2
      minute           = 30
      duration_minutes = 60
    },
    {
      name             = "weekly patching"
      schedule_type    = "Weekly"
      day_of_week      = "Sunday"
      hour             = 4
      duration_minutes = 120
      timezone         = "America/New_York"
    },
  ]

  links = ["https://wiki.example.com/runbooks/web-01"]
  tags  = ["production"]
}
