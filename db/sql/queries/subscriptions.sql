-- name: CreateSubscription :exec
INSERT INTO subscription (
    "referenceId",
    plan,
    status,
    "trialStart",
    "trialEnd"
) VALUES (
    sqlc.arg(reference_id),
    sqlc.arg(plan),
    sqlc.arg(status),
    sqlc.arg(trial_start),
    sqlc.arg(trial_end)
);