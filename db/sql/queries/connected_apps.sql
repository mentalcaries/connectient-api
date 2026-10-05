-- name: GetConnectedApps :many
SELECT provider, connected_account_email, is_connected
FROM connected_apps
WHERE practice_id = sqlc.arg(practice_id);
