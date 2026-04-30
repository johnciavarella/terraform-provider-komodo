data "komodo_build" "api" {
  id = "api" # ID or display name
}

# Reference the build's canonical ID in a deployment.
resource "komodo_deployment" "api" {
  name       = "api"
  server_id  = "Local"
  image_type = "Build"
  image      = data.komodo_build.api.id
}
