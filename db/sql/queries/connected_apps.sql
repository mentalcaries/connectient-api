-- name: GetConnectedApps :many
SELECT * FROM connected_apps WHERE practice_id = sqlc.arg(practice_id);
