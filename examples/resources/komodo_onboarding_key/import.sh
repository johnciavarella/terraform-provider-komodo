# Import an existing onboarding key by its public_key value (the canonical id).
# The PKCS8 private_key cannot be recovered after import — only at create time.
terraform import komodo_onboarding_key.fleet "MCowBQYDK2VuAyEA0123456789abcdefghijklmnopqrstuvwxyz="
