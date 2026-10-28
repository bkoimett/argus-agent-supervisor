-- Argus MVP schema. See docs/prd.md section 12.
-- Row Level Security is enabled on every table per docs/prd.md section 13,
-- policies are added alongside auth wiring in M1.

create table if not exists devices (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references auth.users (id) on delete cascade,
    name text not null,
    paired_at timestamptz not null default now(),
    revoked_at timestamptz
);

create table if not exists projects (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references auth.users (id) on delete cascade,
    name text not null,
    directory text not null,
    agent text not null default 'opencode',
    priority text not null default 'NORMAL',
    created_at timestamptz not null default now()
);

create table if not exists sessions (
    id uuid primary key default gen_random_uuid(),
    project_id uuid not null references projects (id) on delete cascade,
    status text not null default 'STARTING',
    started_at timestamptz not null default now(),
    ended_at timestamptz
);

create table if not exists events (
    id uuid primary key default gen_random_uuid(),
    session_id uuid not null references sessions (id) on delete cascade,
    type text not null,
    payload jsonb,
    created_at timestamptz not null default now()
);

create table if not exists commands (
    id uuid primary key default gen_random_uuid(),
    session_id uuid not null references sessions (id) on delete cascade,
    type text not null,
    payload jsonb,
    status text not null default 'PENDING',
    created_at timestamptz not null default now()
);

create table if not exists notification_preferences (
    user_id uuid primary key references auth.users (id) on delete cascade,
    needs_input boolean not null default true,
    permission_required boolean not null default true,
    completed boolean not null default true,
    crashed boolean not null default true,
    quota_warning boolean not null default true
);

alter table devices enable row level security;
alter table projects enable row level security;
alter table sessions enable row level security;
alter table events enable row level security;
alter table commands enable row level security;
alter table notification_preferences enable row level security;

-- TODO (M1): add per-table RLS policies scoping every row to auth.uid().
