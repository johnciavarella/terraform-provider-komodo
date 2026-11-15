resource "komodo_deployment" "nginx" {
  name        = "nginx"
  description = "Edge proxy"
  custom_name = "edge-nginx" # container name, if different from the deployment name
  server_id   = "Local"
  image_type  = "Image"
  image       = "nginx:1.25"

  network      = "bridge"
  restart_mode = "unless-stopped"

  ports       = "8080:80"
  volumes     = "/srv/nginx/conf.d:/etc/nginx/conf.d:ro"
  environment = "TZ=UTC"

  termination_signal  = "SIGTERM"
  termination_timeout = 30

  tags = ["production"]
}

# Deploy the image produced by a Komodo build, and redeploy whenever it rebuilds.
resource "komodo_deployment" "api" {
  name              = "api"
  server_id         = "Local"
  image_type        = "Build"
  image             = komodo_build.api.id
  redeploy_on_build = true
}
