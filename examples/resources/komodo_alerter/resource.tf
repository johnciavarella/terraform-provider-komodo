# Send every alert to a Discord channel.
resource "komodo_alerter" "discord" {
  name = "discord"
  endpoint = {
    type = "Discord"
    url  = var.discord_webhook_url
  }
}

# Send only server and stack problems to ntfy, and leave one noisy stack out.
resource "komodo_alerter" "ntfy_critical" {
  name = "ntfy-critical"
  endpoint = {
    type  = "Ntfy"
    url   = "https://ntfy.example.com/komodo"
    email = "ops@example.com"
  }

  alert_types = [
    "ServerUnreachable",
    "ServerDisk",
    "StackStateChange",
    "BuildFailed",
  ]

  except_resources = [
    { type = "Stack", id = komodo_stack.scratch.id },
  ]

  tags = ["prod"]
}

# POST alerts as JSON to your own receiver. Created disabled.
resource "komodo_alerter" "custom" {
  name    = "custom-receiver"
  enabled = false
  endpoint = {
    type = "Custom"
    url  = "http://alert-receiver.internal:7000"
  }
}
