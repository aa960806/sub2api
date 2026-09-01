package repository

// legacyMigrationAlias describes one exact SQL migration whose filename was
// renumbered between the legacy SubNexus build and this fork. The checksum is
// SHA256(strings.TrimSpace(SQL)), matching migrations_runner.go. Keeping the
// expected checksum in the map makes a future edit to either historical file
// fail closed instead of silently adopting a different statement set.
type legacyMigrationAlias struct {
	legacyFilename string
	checksum       string
}

// legacyMigrationAliases is intentionally an explicit, narrow allowlist. It
// is not a generic "same checksum" shortcut: only a target filename listed
// here may adopt a matching legacy record. The target and legacy SQL files in
// this list were byte-for-byte identical after TrimSpace during the Batch 0
// audit on 2026-09-01.
var legacyMigrationAliases = map[string]legacyMigrationAlias{
	"158_add_group_peak_rate_multiplier.sql": {
		legacyFilename: "205_add_group_peak_rate_multiplier_compat.sql",
		checksum:       "f3d7f7d41f6c452f94c65ea08b16f64fdb54f0c3528d28905a2bda40fbc58f83",
	},
	"158_enable_grok_media_generation_groups.sql": {
		legacyFilename: "240_enable_grok_media_generation_groups.sql",
		checksum:       "903a5719fb77f6cb468181fcc4c384b90477488f0b0b4ecec10f9c6f94341a21",
	},
	"174_add_usage_log_long_context_billing.sql": {
		legacyFilename: "175_add_usage_log_long_context_billing.sql",
		checksum:       "d4af66d7be685c60a4b47d340f7450a2e3ba385dd9e7b9da8e30d3026b964056",
	},
	"174_add_usage_logs_api_key_latest_ip_index_notx.sql": {
		legacyFilename: "204_add_usage_logs_api_key_latest_ip_index_notx.sql",
		checksum:       "aec8fad2bbb6993340ac93762bd8df62fccc72a41ffeb63181ee8fa58f223a1d",
	},
	"175_add_ops_system_logs_host.sql": {
		legacyFilename: "177_add_ops_system_logs_host.sql",
		checksum:       "176b7d3380d274b71fdae60556b68bc2e5da476e83c90d1e21e3130633deb80a",
	},
	"175a_add_ops_system_logs_host_index_notx.sql": {
		legacyFilename: "177a_add_ops_system_logs_host_index_notx.sql",
		checksum:       "31e54d9ca4d46a9c42de948428ef70fdfca0f790cbac8e78e37d68355947acca",
	},
	"180_audit_logs.sql": {
		legacyFilename: "253_audit_logs.sql",
		checksum:       "c34448727c106179ff72b25f6c0583f56e9985912695678f09457e2f51f9e69b",
	},
	"183_ops_ingress_reject_aggregates.sql": {
		legacyFilename: "182_ops_ingress_reject_aggregates.sql",
		checksum:       "16a2ffccfe0f03451ab5ab6edfe252501d09c5c8927e27d87bbc3f826a5d8871",
	},
	"184_auth_cache_invalidation_outbox.sql": {
		legacyFilename: "183_auth_cache_invalidation_outbox.sql",
		checksum:       "870ff546e67a8c59f99310fab34e2101af71332eea50fff17a6bfb2a4d0fdc7a",
	},
	"185_group_reasoning_effort_policy.sql": {
		legacyFilename: "190_group_reasoning_effort_policy.sql",
		checksum:       "043489b1e240068b949b15ac2f0edb6255503a544b0301cbff16274fb43ca3fd",
	},
	"186_alipay_mobile_precreate_deep_link.sql": {
		legacyFilename: "189_alipay_mobile_precreate_deep_link.sql",
		checksum:       "7b64b8493a6e2896f798e428e7302b23d1e03ee8b58b31566c67dbfbccbfe96a",
	},
	"188_allow_live_usage_request_type.sql": {
		legacyFilename: "193_allow_live_usage_request_type.sql",
		checksum:       "0233dba07a75bd9c740402a64e3af75c2a3884dfc8c4b63145df115e716fd35e",
	},
	"191_passkey_credentials.sql": {
		legacyFilename: "197_passkey_credentials.sql",
		checksum:       "d79e7093f28b1a2ba923da35d7376423683c9a5d21dffd0e581f3c45b5afd817",
	},
	"195_add_usage_log_upstream_model_mismatch_index_notx.sql": {
		legacyFilename: "209_add_usage_log_upstream_model_mismatch_index_notx.sql",
		checksum:       "692f2a75f0c62670b4d68986912bf24eb92f6377ec904d3806ff7d62b0da8355",
	},
	"217_group_video_model_prices.sql": {
		legacyFilename: "235_group_video_model_prices.sql",
		checksum:       "e335f1b68ed1349661fab51bf4669619b7b116df31c1fb974c844b1c8a2f84d3",
	},
	"218_group_audio_voice_pricing.sql": {
		legacyFilename: "236_group_audio_voice_pricing.sql",
		checksum:       "a99ade7d0d464c67bf56814570050cc363ffad64eae2cb1e1ed760065f0b3585",
	},
	"219_group_search_price_per_1k.sql": {
		legacyFilename: "237_group_search_price_per_1k.sql",
		checksum:       "430c2e3595342fe22c59e9676e9b18ea376f076324b77174a21e6f181f57f4b5",
	},
	"221_group_model_pricing.sql": {
		legacyFilename: "239_group_model_pricing.sql",
		checksum:       "1ba1940ef00e9a3831ec2afa90495660daba9191bfec311567228ed2a6e180e3",
	},
	"222_group_usage_daily_rollups.sql": {
		legacyFilename: "241_group_usage_daily_rollups.sql",
		checksum:       "d1dea80dd961e8f4016ce95dc413ee28344adf6fce9964427452a95cb7dfc16f",
	},
	"223_group_usage_rollup_timezone.sql": {
		legacyFilename: "242_group_usage_rollup_timezone.sql",
		checksum:       "68ab749e218a893f7c54d856510c8f8eb462d7ce4d8e300d5fa8b6a17cf631cc",
	},
	"225_backfill_codex_fingerprint_seed.sql": {
		legacyFilename: "244_backfill_codex_fingerprint_seed.sql",
		checksum:       "bd8d6dff505e417eee69a2da300aa1df06e832fd668c7848f06944c7c0c3fd26",
	},
	"225_channel_model_time_pricing.sql": {
		legacyFilename: "245_channel_model_time_pricing.sql",
		checksum:       "23f0a4da20f2f78f385e9f1cd1ed57db1a31b99b1ba54b0f498985e3a66647b1",
	},
	"226_add_usage_log_effective_model_indexes_notx.sql": {
		legacyFilename: "246_add_usage_log_effective_model_indexes_notx.sql",
		checksum:       "8b85d3a071e0822f75244cc242dbf56df83ac85f7fdca3940590185335b5c239",
	},
}
