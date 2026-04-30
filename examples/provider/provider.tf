terraform {
  required_providers {
    komodo = {
      source  = "johnciavarella/komodo"
      version = "~> 2.0"
    }
  }
}

# Credentials and address can also be supplied via the environment variables
# KOMODO_ADDRESS, KOMODO_API_KEY, and KOMODO_API_SECRET.
provider "komodo" {
  base_url   = "https://komodo.example.com"
  api_key    = var.komodo_api_key
  api_secret = var.komodo_api_secret

  # Set to true only for local-test instances behind a self-signed cert.
  insecure_skip_verify = false
}
