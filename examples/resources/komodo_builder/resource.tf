resource "komodo_builder" "default" {
  name      = "default-builder"
  server_id = "Local" # or a komodo_server.<name>.id
}
