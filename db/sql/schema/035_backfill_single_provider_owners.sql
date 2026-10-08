-- +goose Up
WITH candidates AS (
    SELECT DISTINCT ON (practice.id)
        practice.id AS practice_id,
        owner.first_name,
        owner.last_name,
        COALESCE(NULLIF(BTRIM(practice.specialty), ''), 'General') AS specialty,
        MD5('connectient-owner-provider:' || owner.id::text)::uuid AS provider_id
    FROM practices AS practice
    JOIN users AS owner
        ON owner.practice_id = practice.id
        AND owner.role = 'owner'
        AND owner.is_active = TRUE
        AND owner.deleted_at IS NULL
    WHERE practice.has_multiple_providers = FALSE
      AND NOT EXISTS (
          SELECT 1
          FROM practice_provider
          WHERE practice_provider.practice_id = practice.id
      )
    ORDER BY practice.id, owner.created_at, owner.id
)
INSERT INTO provider (id, first_name, last_name, specialty)
SELECT provider_id, first_name, last_name, specialty
FROM candidates;

WITH candidates AS (
    SELECT DISTINCT ON (practice.id)
        practice.id AS practice_id,
        MD5('connectient-owner-provider:' || owner.id::text)::uuid AS provider_id
    FROM practices AS practice
    JOIN users AS owner
        ON owner.practice_id = practice.id
        AND owner.role = 'owner'
        AND owner.is_active = TRUE
        AND owner.deleted_at IS NULL
    WHERE practice.has_multiple_providers = FALSE
      AND NOT EXISTS (
          SELECT 1
          FROM practice_provider
          WHERE practice_provider.practice_id = practice.id
      )
    ORDER BY practice.id, owner.created_at, owner.id
)
INSERT INTO practice_provider (practice_id, provider_id, is_main)
SELECT practice_id, provider_id, TRUE
FROM candidates;

-- +goose Down
DELETE FROM practice_provider AS practice_provider
USING users AS owner
WHERE practice_provider.practice_id = owner.practice_id
  AND owner.role = 'owner'
  AND practice_provider.provider_id = MD5('connectient-owner-provider:' || owner.id::text)::uuid;

DELETE FROM provider AS provider
USING users AS owner
WHERE provider.id = MD5('connectient-owner-provider:' || owner.id::text)::uuid
  AND NOT EXISTS (
      SELECT 1
      FROM practice_provider
      WHERE practice_provider.provider_id = provider.id
  );
