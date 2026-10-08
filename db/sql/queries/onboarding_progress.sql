-- name: ListOnboardingProgress :many
SELECT walkthrough_key, seen_at, dismissed_at, completed_at
FROM onboarding_progress
WHERE user_id = sqlc.arg(user_id);

-- name: UpsertOnboardingProgress :one
INSERT INTO onboarding_progress (
    user_id, walkthrough_key, seen_at, dismissed_at, completed_at, updated_at
) VALUES (
    sqlc.arg(user_id),
    sqlc.arg(walkthrough_key),
    CASE WHEN sqlc.arg(set_seen)::boolean THEN NOW() ELSE NULL END,
    CASE WHEN sqlc.arg(set_dismissed)::boolean THEN NOW() ELSE NULL END,
    CASE WHEN sqlc.arg(set_completed)::boolean THEN NOW() ELSE NULL END,
    NOW()
)
ON CONFLICT (user_id, walkthrough_key) DO UPDATE SET
    seen_at = CASE WHEN sqlc.arg(set_seen)::boolean THEN NOW() ELSE onboarding_progress.seen_at END,
    dismissed_at = CASE WHEN sqlc.arg(set_dismissed)::boolean THEN NOW() ELSE onboarding_progress.dismissed_at END,
    completed_at = CASE WHEN sqlc.arg(set_completed)::boolean THEN NOW() ELSE onboarding_progress.completed_at END,
    updated_at = NOW()
RETURNING walkthrough_key, seen_at, dismissed_at, completed_at;
