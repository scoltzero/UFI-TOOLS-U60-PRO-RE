# Observed compatibility routes

The following route families were recovered as public protocol facts and are
registered by the compatibility router. A registered route may be fully
implemented, simulated/persisted, or explicitly return `501` until its
privileged adapter has been configured.

## Core/device

`/api/baseDeviceInfo`, `/api/connInfo`, `/api/device_id`, `/api/device_uuid`,
`/api/version_info`, `/api/SELinux`, `/api/usb_status`, `/api/wifi/qrcode`,
`/api/goform/goform_get_cmd_process`, `/api/goform/goform_set_cmd_process`.

## Configuration/authentication

`/api/need_token`, `/api/is_weak_token`, `/api/set_token`, `/api/accept_terms`,
`/api/get_theme`, `/api/set_theme`, `/api/set_nickname`, `/api/get_res_server`,
`/api/set_res_server`, `/api/get_log_status`, `/api/set_log_status`,
`/api/log_clear`, `/api/set_wakelock_status`, `/api/update_admin_pwd`.

## Device features

`/api/apn/*`, `/api/adb/status`, `/api/adb/mode`, `/api/adb_alive`,
`/api/ttyd/status`, `/api/ttyd/start`, `/api/ttyd/stop`, `/api/hasTTYD`,
`/api/time/status`, `/api/time/sync`, `/api/time/config`, `/api/hosts`,
`/api/hosts_restore`, `/api/run_shell`, `/api/speedtest`.

## State and automation

`/api/add_task`, `/api/get_task`, `/api/list_tasks`, `/api/remove_task`,
`/api/clear_task`, `/api/cellularUsage`, `/api/power_status_forward_enabled`,
and the `/api/sms_forward_*` family.

## Plugins/uploads/updates

`/api/get_custom_head`, `/api/set_custom_head`, `/api/plugins_store`,
`/api/plugin/*`, `/api/upload_file`, `/api/upload_img`, `/api/delete_img`,
`/api/list_uploads`, `/api/delete_all_uploads_data`, `/api/uploads/*`,
`/api/check_update`, `/api/download_update`, `/api/download_update_status`,
`/api/install_update`, `/api/auto_update/config`, and
`/api/auto_update/check_now`.

The route registration test is the executable source of truth. Exact request
and response schemas remain evidence-gated where black-box fixtures have not
yet established them.

