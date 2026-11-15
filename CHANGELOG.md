# Changelog

## 3.0.0 (November 14, 2026)

BREAKING CHANGES:

No attribute was removed, renamed or changed type, and existing state loads without an upgrade step. These changes alter behaviour for existing configurations:

* resource/komodo_deployment: The new `custom_name` attribute defaults to `""` and is enforced. A custom container name set in the Komodo UI is cleared on the next apply, and Komodo renames the running container back to the deployment name. Add the current value to your config before upgrading.
* resource/komodo_deployment: The new `redeploy_on_build` attribute defaults to `false` and is enforced. A value of `true` set in the Komodo UI is turned off on the next apply unless it is added to your config.
* resource/komodo_build: The new `secret_args` attribute defaults to `""` and is enforced. Secret build arguments set in the Komodo UI are removed on the next apply unless they are added to your config.
* resource/komodo_builder: `server_id` is only sent to Komodo when it changes. Previously every update resent it.

NOTES:

* Run `terraform plan` after upgrading and check `komodo_deployment` and `komodo_build` resources for the three attributes above before applying.
* The following new attributes are unmanaged unless configured: nothing is sent, and the value in Komodo is not read into state. Set `[]` or `""` to clear one. `terraform import` does not read them until they are configured.
  * `description` and `template` on `komodo_server`, `komodo_stack`, `komodo_deployment`, `komodo_build`, `komodo_repo`, `komodo_builder`, `komodo_alerter` and `komodo_procedure`
  * `komodo_server.maintenance_windows`
  * `komodo_stack.config_files` and `komodo_stack.auto_update_skip_services`
  * `komodo_build.extra_args`
* resource/komodo_api_key: Keys already in state take the new `expires` default (`0`, no expiry) in place and are not replaced.
* resource/komodo_server: `send_version_mismatch_alerts` has no provider default, so existing servers keep Komodo's own default (`true`).
* The new resources and fields were written against the Komodo 2.3.3 API. Configuring a field on a Komodo version that does not have it is likely to fail the apply.

FEATURES:

* **New Resource:** `komodo_variable`: global variables, including secret ones, referenced as `[[NAME]]`.
* **New Resource:** `komodo_alerter`: routes alerts to Custom, Slack, Discord, Ntfy or Pushover endpoints, with alert-type and resource filters.
* **New Resource:** `komodo_procedure`: stages of parallel executions with schedule, webhook and alert settings.
* **New Data Source:** `komodo_variable`
* **New Data Source:** `komodo_alerter`
* **New Data Source:** `komodo_procedure`

ENHANCEMENTS:

* resource/komodo_server: Add `send_version_mismatch_alerts` and `maintenance_windows`.
* resource/komodo_stack: Add `config_files` and `auto_update_skip_services`.
* resource/komodo_deployment: Add `custom_name` and `redeploy_on_build`.
* resource/komodo_build: Add `extra_args`, `secret_args` and `version`.
* resource/komodo_api_key: Add `expires`. Changing it replaces the key.
* resource/komodo_server, komodo_stack, komodo_deployment, komodo_build, komodo_repo, komodo_builder: Add `description` and `template`.
* data-source/komodo_stack: Add `ignore_services`.
* Plan-time validation for `komodo_deployment.custom_name`, `komodo_stack.config_files[].requires`, `komodo_server.maintenance_windows`, `komodo_build.version`, `komodo_variable.name` and `value`, alerter endpoint, alert and resource types, procedure `schedule_format` and execution `params`, and the names of alerters and procedures.

BUG FIXES:

* resource/komodo_builder: Read `server_id` correctly on Komodo 2.3, which returns a builder's servers as a `server_ids` list. Previously it read back empty and every plan showed a change.
* resource/komodo_builder: A rename or tag change no longer drops the extra servers of a multi-server builder.

## 2.0.1 (April 30, 2026)

* Documentation: example usage on every resource and data source page.

## 2.0.0 (April 30, 2026)

* Upgrade the provider to Komodo Core v2.1.x. See "Upgrading from 1.x" in the README.
