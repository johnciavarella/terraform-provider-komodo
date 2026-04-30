# Look up a server by ID or name. Komodo's auto-registered Periphery is
# reachable as "Local".
data "komodo_server" "primary" {
  id = "Local"
}

output "primary_address" {
  value = data.komodo_server.primary.address
}
