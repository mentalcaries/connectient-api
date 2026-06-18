-- +goose Up
CREATE TABLE IF NOT EXISTS subscription (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plan TEXT NOT NULL,
    "referenceId" TEXT NOT NULL,
    "stripeCustomerId" TEXT,
    "stripeSubscriptionId" TEXT,
    status TEXT NOT NULL,
    "periodStart" TIMESTAMPTZ,
    "periodEnd" TIMESTAMPTZ,
    "trialStart" TIMESTAMPTZ,
    "trialEnd" TIMESTAMPTZ,
    "cancelAtPeriodEnd" BOOLEAN,
    "cancelAt" TIMESTAMPTZ,
    "canceledAt" TIMESTAMPTZ,
    "endedAt" TIMESTAMPTZ,
    seats INTEGER,
    "billingInterval" TEXT,
    "stripeScheduleId" TEXT
);

-- +goose Down
-- Intentionally no-op: this table is owned and managed by Better Auth's
-- Stripe plugin, not Goose. Do not drop it here.