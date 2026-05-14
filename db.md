-- WARNING: This schema is for context only and is not meant to be run.
-- Table order and constraints may not be valid for execution.

CREATE TABLE public.Appointments (
  id uuid NOT NULL DEFAULT gen_random_uuid(),
  created_at timestamp with time zone DEFAULT now(),
  first_name character varying NOT NULL,
  last_name character varying NOT NULL,
  email character varying NOT NULL,
  mobile_phone character varying NOT NULL,
  requested_date date,
  is_emergency boolean DEFAULT false,
  description text,
  appointment_type text,
  is_scheduled boolean DEFAULT false,
  scheduled_date date,
  created_by uuid,
  scheduled_by uuid,
  is_cancelled boolean DEFAULT false,
  requested_time text DEFAULT ''::text,
  scheduled_time time without time zone,
  practice_id uuid,
  modified_at timestamp with time zone,
  provider_id uuid,
  location_id uuid,
  duration_minutes integer,
  patient_id uuid,
  CONSTRAINT Appointments_pkey PRIMARY KEY (id),
  CONSTRAINT Appointments_created_by_fkey FOREIGN KEY (created_by) REFERENCES auth.users(id),
  CONSTRAINT Appointments_scheduled_by_fkey FOREIGN KEY (scheduled_by) REFERENCES public.Users(id),
  CONSTRAINT Appointments_provider_id_fkey FOREIGN KEY (provider_id) REFERENCES public.Provider(id),
  CONSTRAINT Appointments_location_id_fkey FOREIGN KEY (location_id) REFERENCES public.practice_locations(id),
  CONSTRAINT Appointments_practice_id_fkey FOREIGN KEY (practice_id) REFERENCES public.Practice(id),
  CONSTRAINT Appointments_patient_id_fkey FOREIGN KEY (patient_id) REFERENCES public.patients(id)
);
CREATE TABLE public.Practice (
  created_at timestamp with time zone DEFAULT now(),
  name text NOT NULL,
  city text NOT NULL,
  phone text,
  email text,
  practice_code text NOT NULL UNIQUE,
  logo text,
  id uuid NOT NULL DEFAULT gen_random_uuid(),
  street_address text,
  facebook text,
  instagram text,
  website text,
  has_multiple_providers boolean NOT NULL DEFAULT false,
  specialty text,
  is_suspended boolean NOT NULL DEFAULT false,
  practice_category text NOT NULL CHECK (practice_category = ANY (ARRAY['dental'::text, 'medical'::text, 'optometry'::text, 'physiotherapy'::text])),
  CONSTRAINT Practice_pkey PRIMARY KEY (id)
);
CREATE TABLE public.PracticeProvider (
  id uuid NOT NULL DEFAULT gen_random_uuid(),
  practice_id uuid NOT NULL,
  provider_id uuid NOT NULL,
  created_at timestamp with time zone DEFAULT now(),
  is_main boolean NOT NULL DEFAULT false,
  CONSTRAINT PracticeProvider_pkey PRIMARY KEY (id),
  CONSTRAINT PracticeProvider_practice_id_fkey FOREIGN KEY (practice_id) REFERENCES public.Practice(id),
  CONSTRAINT PracticeProvider_provider_id_fkey FOREIGN KEY (provider_id) REFERENCES public.Provider(id)
);
CREATE TABLE public.Provider (
  id uuid NOT NULL DEFAULT gen_random_uuid(),
  first_name text NOT NULL,
  last_name text NOT NULL,
  title text,
  specialty text NOT NULL DEFAULT 'general'::text,
  created_at timestamp with time zone DEFAULT now(),
  CONSTRAINT Provider_pkey PRIMARY KEY (id)
);
CREATE TABLE public.Users (
  id uuid NOT NULL,
  created_at timestamp with time zone DEFAULT now(),
  first_name text NOT NULL,
  last_name text NOT NULL,
  mobile_phone text UNIQUE,
  email character varying,
  practice_id uuid,
  role text DEFAULT 'staff'::text CHECK (role = ANY (ARRAY['owner'::text, 'admin'::text, 'staff'::text])),
  org_role text,
  is_active boolean NOT NULL DEFAULT true,
  invited_by uuid,
  deleted_at timestamp with time zone,
  avatar_url text,
  whatsapp_notifications_enabled boolean NOT NULL DEFAULT true,
  terms_agreed_at timestamp with time zone,
  CONSTRAINT Users_pkey PRIMARY KEY (id),
  CONSTRAINT Users_practice_id_fkey FOREIGN KEY (practice_id) REFERENCES public.Practice(id),
  CONSTRAINT Users_invited_by_fkey FOREIGN KEY (invited_by) REFERENCES public.Users(id)
);
CREATE TABLE public.account (
  id text NOT NULL DEFAULT gen_random_uuid(),
  accountId text NOT NULL,
  providerId text NOT NULL,
  userId uuid NOT NULL,
  accessToken text,
  refreshToken text,
  idToken text,
  accessTokenExpiresAt timestamp with time zone,
  refreshTokenExpiresAt timestamp with time zone,
  scope text,
  password text,
  createdAt timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updatedAt timestamp with time zone NOT NULL,
  CONSTRAINT account_pkey PRIMARY KEY (id),
  CONSTRAINT account_userId_fkey FOREIGN KEY (userId) REFERENCES public.user(id)
);
CREATE TABLE public.appointment_calendar_events (
  id uuid NOT NULL DEFAULT gen_random_uuid(),
  appointment_id uuid NOT NULL,
  provider text NOT NULL DEFAULT 'google_calendar'::text,
  external_event_id text NOT NULL,
  created_at timestamp with time zone NOT NULL DEFAULT now(),
  updated_at timestamp with time zone NOT NULL DEFAULT now(),
  CONSTRAINT appointment_calendar_events_pkey PRIMARY KEY (id),
  CONSTRAINT appointment_calendar_events_appointment_id_fkey FOREIGN KEY (appointment_id) REFERENCES public.Appointments(id)
);
CREATE TABLE public.connected_apps (
  id uuid NOT NULL DEFAULT gen_random_uuid(),
  practice_id uuid NOT NULL,
  provider text NOT NULL,
  connected_account_email text,
  access_token text,
  refresh_token text,
  token_expires_at timestamp with time zone,
  is_connected boolean NOT NULL DEFAULT true,
  last_error text,
  created_at timestamp with time zone NOT NULL DEFAULT now(),
  updated_at timestamp with time zone NOT NULL DEFAULT now(),
  CONSTRAINT connected_apps_pkey PRIMARY KEY (id),
  CONSTRAINT connected_apps_practice_id_fkey FOREIGN KEY (practice_id) REFERENCES public.Practice(id)
);
CREATE TABLE public.notification_log (
  id uuid NOT NULL DEFAULT gen_random_uuid(),
  practice_id uuid NOT NULL,
  channel text NOT NULL CHECK (channel = ANY (ARRAY['email'::text, 'whatsapp'::text])),
  notification_type text NOT NULL,
  sent_at timestamp with time zone NOT NULL DEFAULT now(),
  CONSTRAINT notification_log_pkey PRIMARY KEY (id),
  CONSTRAINT notification_log_practice_id_fkey FOREIGN KEY (practice_id) REFERENCES public.Practice(id)
);
CREATE TABLE public.patient_registration_data (
  id uuid NOT NULL DEFAULT gen_random_uuid(),
  registration_id uuid NOT NULL,
  form_version text NOT NULL,
  form_data jsonb NOT NULL,
  submitted_at timestamp with time zone NOT NULL DEFAULT now(),
  CONSTRAINT patient_registration_data_pkey PRIMARY KEY (id),
  CONSTRAINT patient_registration_data_registration_id_fkey FOREIGN KEY (registration_id) REFERENCES public.patient_registrations(id)
);
CREATE TABLE public.patient_registrations (
  id uuid NOT NULL DEFAULT gen_random_uuid(),
  practice_id uuid NOT NULL,
  appointment_id uuid,
  patient_name text NOT NULL,
  patient_email text NOT NULL,
  token text NOT NULL UNIQUE,
  token_expires_at timestamp with time zone NOT NULL,
  status text NOT NULL DEFAULT 'pending'::text CHECK (status = ANY (ARRAY['pending'::text, 'completed'::text, 'expired'::text])),
  sent_by_user_id uuid NOT NULL,
  sent_at timestamp with time zone,
  completed_at timestamp with time zone,
  deleted_at timestamp with time zone,
  created_at timestamp with time zone NOT NULL DEFAULT now(),
  patient_id uuid,
  CONSTRAINT patient_registrations_pkey PRIMARY KEY (id),
  CONSTRAINT patient_registrations_practice_id_fkey FOREIGN KEY (practice_id) REFERENCES public.Practice(id),
  CONSTRAINT patient_registrations_appointment_id_fkey FOREIGN KEY (appointment_id) REFERENCES public.Appointments(id),
  CONSTRAINT patient_registrations_sent_by_user_id_fkey FOREIGN KEY (sent_by_user_id) REFERENCES public.Users(id),
  CONSTRAINT patient_registrations_patient_id_fkey FOREIGN KEY (patient_id) REFERENCES public.patients(id)
);
CREATE TABLE public.patients (
  id uuid NOT NULL DEFAULT gen_random_uuid(),
  practice_id uuid NOT NULL,
  first_name text NOT NULL,
  last_name text NOT NULL,
  email text,
  mobile_phone text,
  home_phone text,
  date_of_birth date,
  address_line_1 text,
  address_line_2 text,
  city text,
  email_consent boolean NOT NULL DEFAULT true,
  whatsapp_consent boolean NOT NULL DEFAULT true,
  emergency_contact_name text,
  emergency_contact_phone text,
  notes text,
  created_at timestamp with time zone NOT NULL DEFAULT now(),
  updated_at timestamp with time zone NOT NULL DEFAULT now(),
  CONSTRAINT patients_pkey PRIMARY KEY (id),
  CONSTRAINT patients_practice_id_fkey FOREIGN KEY (practice_id) REFERENCES public.Practice(id)
);
CREATE TABLE public.practice_invites (
  id uuid NOT NULL DEFAULT gen_random_uuid(),
  practice_id uuid NOT NULL,
  email text NOT NULL,
  first_name text NOT NULL,
  last_name text NOT NULL,
  org_role text,
  role text NOT NULL CHECK (role = ANY (ARRAY['admin'::text, 'staff'::text])),
  invited_by uuid NOT NULL,
  token text NOT NULL UNIQUE,
  token_expires_at timestamp with time zone NOT NULL,
  accepted_at timestamp with time zone,
  created_at timestamp with time zone NOT NULL DEFAULT now(),
  CONSTRAINT practice_invites_pkey PRIMARY KEY (id),
  CONSTRAINT practice_invites_practice_id_fkey FOREIGN KEY (practice_id) REFERENCES public.Practice(id),
  CONSTRAINT practice_invites_invited_by_fkey FOREIGN KEY (invited_by) REFERENCES public.Users(id)
);
CREATE TABLE public.practice_locations (
  id uuid NOT NULL DEFAULT gen_random_uuid(),
  practice_id uuid NOT NULL,
  name text NOT NULL,
  address text NOT NULL,
  is_active boolean NOT NULL DEFAULT true,
  sort_order integer NOT NULL DEFAULT 0,
  created_at timestamp with time zone NOT NULL DEFAULT now(),
  deleted_at timestamp with time zone,
  CONSTRAINT practice_locations_pkey PRIMARY KEY (id),
  CONSTRAINT practice_locations_practice_id_fkey FOREIGN KEY (practice_id) REFERENCES public.Practice(id)
);
CREATE TABLE public.practice_settings (
  id uuid NOT NULL DEFAULT gen_random_uuid(),
  practice_id uuid NOT NULL UNIQUE,
  dental_history_enabled boolean NOT NULL DEFAULT false,
  tmj_history_enabled boolean NOT NULL DEFAULT false,
  multiple_locations_enabled boolean NOT NULL DEFAULT false,
  custom_form_sections jsonb,
  created_at timestamp with time zone NOT NULL DEFAULT now(),
  updated_at timestamp with time zone NOT NULL DEFAULT now(),
  optometry_history_enabled boolean NOT NULL DEFAULT false,
  physiotherapy_history_enabled boolean NOT NULL DEFAULT false,
  theme text NOT NULL DEFAULT 'default'::text,
  theme_colors jsonb,
  CONSTRAINT practice_settings_pkey PRIMARY KEY (id),
  CONSTRAINT practice_settings_practice_id_fkey FOREIGN KEY (practice_id) REFERENCES public.Practice(id)
);
CREATE TABLE public.procedure_types (
  id uuid NOT NULL DEFAULT gen_random_uuid(),
  practice_id uuid NOT NULL,
  name text NOT NULL,
  value text NOT NULL,
  is_active boolean NOT NULL DEFAULT true,
  is_default boolean NOT NULL DEFAULT false,
  sort_order integer NOT NULL DEFAULT 0,
  created_at timestamp with time zone NOT NULL DEFAULT now(),
  is_primary boolean NOT NULL DEFAULT false,
  deleted_at timestamp with time zone,
  CONSTRAINT procedure_types_pkey PRIMARY KEY (id),
  CONSTRAINT procedure_types_practice_id_fkey FOREIGN KEY (practice_id) REFERENCES public.Practice(id)
);
CREATE TABLE public.session (
  id uuid NOT NULL DEFAULT gen_random_uuid(),
  expiresAt timestamp with time zone NOT NULL,
  token text NOT NULL UNIQUE,
  createdAt timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updatedAt timestamp with time zone NOT NULL,
  ipAddress text,
  userAgent text,
  userId uuid NOT NULL,
  CONSTRAINT session_pkey PRIMARY KEY (id),
  CONSTRAINT session_userId_fkey FOREIGN KEY (userId) REFERENCES public.user(id)
);
CREATE TABLE public.subscription (
  id uuid NOT NULL DEFAULT gen_random_uuid(),
  plan text NOT NULL,
  referenceId text NOT NULL,
  stripeCustomerId text,
  stripeSubscriptionId text,
  status text NOT NULL,
  periodStart timestamp with time zone,
  periodEnd timestamp with time zone,
  trialStart timestamp with time zone,
  trialEnd timestamp with time zone,
  cancelAtPeriodEnd boolean,
  cancelAt timestamp with time zone,
  canceledAt timestamp with time zone,
  endedAt timestamp with time zone,
  seats integer,
  billingInterval text,
  stripeScheduleId text,
  CONSTRAINT subscription_pkey PRIMARY KEY (id)
);
CREATE TABLE public.subscriptions (
  id uuid NOT NULL DEFAULT gen_random_uuid(),
  practice_id uuid NOT NULL,
  stripe_customer_id text,
  stripe_subscription_id text,
  status text NOT NULL DEFAULT 'trialing'::text CHECK (status = ANY (ARRAY['trialing'::text, 'active'::text, 'past_due'::text, 'cancelled'::text])),
  trial_ends_at timestamp with time zone NOT NULL,
  current_period_ends_at timestamp with time zone,
  subscribed_at timestamp with time zone,
  cancelled_at timestamp with time zone,
  created_at timestamp with time zone NOT NULL DEFAULT now(),
  updated_at timestamp with time zone NOT NULL DEFAULT now(),
  plan text NOT NULL DEFAULT 'team'::text CHECK (plan = ANY (ARRAY['solo'::text, 'team'::text, 'practice'::text])),
  CONSTRAINT subscriptions_pkey PRIMARY KEY (id),
  CONSTRAINT subscriptions_practice_id_fkey FOREIGN KEY (practice_id) REFERENCES public.Practice(id)
);
CREATE TABLE public.user (
  id uuid NOT NULL DEFAULT gen_random_uuid(),
  name text NOT NULL,
  email text NOT NULL UNIQUE,
  emailVerified boolean NOT NULL,
  image text,
  createdAt timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updatedAt timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  stripeCustomerId text,
  CONSTRAINT user_pkey PRIMARY KEY (id)
);
CREATE TABLE public.verification (
  id uuid NOT NULL DEFAULT gen_random_uuid(),
  identifier text NOT NULL,
  value text NOT NULL,
  expiresAt timestamp with time zone NOT NULL,
  createdAt timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updatedAt timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT verification_pkey PRIMARY KEY (id)
);