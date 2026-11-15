# Nightly: pull the config repo, then redeploy any stack whose files changed.
resource "komodo_procedure" "nightly_deploy" {
  name              = "nightly-deploy"
  schedule_format   = "Cron"
  schedule          = "0 0 2 * * ?" # seconds minutes hours day month weekday
  schedule_timezone = "America/New_York"
  schedule_alert    = false # only alert on failure

  stages = [
    {
      name = "Pull config"
      executions = [{
        type   = "PullRepo"
        params = jsonencode({ repo = komodo_repo.config.id })
      }]
    },
    {
      name = "Deploy changed stacks"
      # Executions in one stage run in parallel.
      executions = [
        {
          type   = "DeployStackIfChanged"
          params = jsonencode({ stack = komodo_stack.app.id })
        },
        {
          type   = "BatchDeployStackIfChanged"
          params = jsonencode({ pattern = "media-*" })
        },
      ]
    },
    {
      name    = "Prune"
      enabled = false # kept in config but skipped
      executions = [{
        type   = "PruneImages"
        params = jsonencode({ server = komodo_server.main.id })
      }]
    },
  ]

  tags = ["system"]
}

# Plain-English schedule; an execution without params.
resource "komodo_procedure" "backup" {
  name     = "backup-core-database"
  schedule = "every day at 1am"

  stages = [{
    name       = "Backup"
    executions = [{ type = "BackupCoreDatabase" }]
  }]
}
