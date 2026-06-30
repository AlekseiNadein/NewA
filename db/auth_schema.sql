-- Auth contour: companies, users, license quotas.
-- Applied to APP_AUTH_DATABASE_URL (schema auth).

CREATE SCHEMA IF NOT EXISTS auth;

CREATE TABLE IF NOT EXISTS auth.companies (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS auth.users (
    id TEXT PRIMARY KEY,
    company_id TEXT NOT NULL REFERENCES auth.companies(id) ON DELETE RESTRICT,
    email TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    authorized BOOLEAN NOT NULL DEFAULT false,
    is_administrator BOOLEAN NOT NULL DEFAULT false,
    is_super_administrator BOOLEAN NOT NULL DEFAULT false,
    password_hash TEXT NOT NULL,
    password_salt TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_auth_users_company_id ON auth.users(company_id);
CREATE INDEX IF NOT EXISTS idx_auth_users_email_lower ON auth.users (lower(email));

CREATE TABLE IF NOT EXISTS auth.company_licenses (
    company_id TEXT NOT NULL REFERENCES auth.companies(id) ON DELETE CASCADE,
    subsection_id TEXT NOT NULL,
    available INTEGER NOT NULL DEFAULT 0 CHECK (available >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (company_id, subsection_id)
);
