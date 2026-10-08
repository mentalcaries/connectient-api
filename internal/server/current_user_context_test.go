package server

import (
	"slices"
	"testing"
	"time"
)

func TestSubscriptionContext(t *testing.T) {
	now := time.Date(2026, time.October, 6, 12, 0, 0, 0, time.UTC)
	trialing, active, pastDue, canceled := "trialing", "active", "past_due", "canceled"
	pro := "pro"
	for _, tc := range []struct {
		name                           string
		status                         *string
		trialEnd, periodEnd            *time.Time
		wantStatus                     string
		active, expired, grace, access bool
	}{
		{name: "missing subscription", wantStatus: "none"},
		{name: "active trial", status: &trialing, trialEnd: timePointer(now.Add(6 * 24 * time.Hour)), wantStatus: "trialing", active: true, access: true},
		{name: "expired trial in grace", status: &trialing, trialEnd: timePointer(now.Add(-24 * time.Hour)), wantStatus: "expired", expired: true, grace: true, access: true},
		{name: "expired active in grace", status: &active, periodEnd: timePointer(now.Add(-24 * time.Hour)), wantStatus: "expired", expired: true, grace: true, access: true},
		{name: "past due after grace", status: &pastDue, periodEnd: timePointer(now.Add(-31 * 24 * time.Hour)), wantStatus: "past_due", expired: true},
		{name: "canceled without expiry", status: &canceled, wantStatus: "canceled", expired: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := computeSubscriptionContext(tc.status, &pro, tc.trialEnd, tc.periodEnd, nil, now)
			if got.Status != tc.wantStatus || got.IsActive != tc.active || got.IsExpired != tc.expired ||
				got.IsInGracePeriod != tc.grace || got.CanAccessBookings != tc.access ||
				got.CanAccessRegistrations != tc.access || got.CanAccessCalendar != tc.access {
				t.Errorf("unexpected subscription context: %+v", got)
			}
		})
	}
}

func TestContextPermissions(t *testing.T) {
	owner, admin, staff := "owner", "admin", "staff"
	active := &SubscriptionContext{CanAccessBookings: true, CanAccessRegistrations: true, CanAccessCalendar: true}
	expired := &SubscriptionContext{}
	for _, tc := range []struct {
		name         string
		role         *string
		member       bool
		subscription *SubscriptionContext
		want         []string
	}{
		{name: "owner active", role: &owner, member: true, subscription: active, want: []string{"bookings:access", "registrations:access", "calendar:access", "settings:manage", "team:manage", "billing:manage", "connected_apps:manage"}},
		{name: "admin expired recovery", role: &admin, member: true, subscription: expired, want: []string{"billing:manage"}},
		{name: "staff active", role: &staff, member: true, subscription: active, want: []string{"bookings:access", "registrations:access", "calendar:access"}},
		{name: "staff expired", role: &staff, member: true, subscription: expired, want: []string{}},
		{name: "revoked owner", role: &owner, subscription: active, want: []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := contextPermissions(tc.role, tc.member, tc.subscription); !slices.Equal(got, tc.want) {
				t.Errorf("permissions = %v, want %v", got, tc.want)
			}
		})
	}
}

func timePointer(value time.Time) *time.Time { return &value }
