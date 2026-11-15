resource "komodo_builder" "default" {
  name        = "default-builder"
  description = "Builds images on the Komodo host"
  server_id   = "Local" # or a komodo_server.<name>.id
}
