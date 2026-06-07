# V1 TDD Scaffold

High-level outside-in testing plan for the V1 meta CLI. This is intentionally not a full implementation.

## Test strategy

Drive implementation from acceptance tests that execute the real `plane-cli` binary.

Use:

- temp `HOME`
- temp `XDG_CONFIG_HOME`
- controlled environment variables
- fake Plane HTTP server for networked cases
- JSON assertions, not prose scraping

Unskip one scenario at a time in `v1_meta_cli_acceptance_test.go`, watch it fail for the right reason, then implement the smallest slice to make it pass.

## Scenario order

1. `version_json_contract`
2. `config_get_redacts_secrets`
3. `config_set_rejects_secret_keys`
4. `auth_status_missing_config_is_typed_and_offline`
5. `auth_status_validates_pat_with_fake_plane`
6. `doctor_for_agent_is_actionable`
7. `resolve_readable_work_item_id`
8. `resolve_invalid_reference_is_typed`

## Verification focus

The valuable assertions are:

- exit code is correct
- stdout is valid JSON in JSON mode
- stderr has diagnostics only
- secrets never appear in stdout/stderr
- incomplete local config fails before network access
- fake server receives the expected request path and auth material
- error envelopes use stable codes and actionable fixes

## Not in this scaffold

- real Plane workspace tests
- persistent cache behavior
- OAuth
- core workflow commands
- operation apply/verify/revert
