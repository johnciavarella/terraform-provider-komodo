resource "komodo_deployment" "nginx" {
  name       = "nginx"
  server_id  = "Local"
  image_type = "Image"
  image      = "nginx:1.25"

  network      = "bridge"
  restart_mode = "unless-stopped"

  ports       = "8080:80"
  volumes     = "/srv/nginx/conf.d:/etc/nginx/conf.d:ro"
  environment = "TZ=UTC"

  termination_signal  = "SIGTERM"
  termination_timeout = 30

  tags = ["production"]
}
