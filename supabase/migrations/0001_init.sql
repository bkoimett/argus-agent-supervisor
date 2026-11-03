-- Argus MVP schema. See docs/prd.md section 12.
-- Row Level Security is enabled on every table per docs/prd.md section 13,
-- policies are added alongside auth wiring in M1.

create table devices (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references auth.users (id) on delete cascade,
    name text not null,
    paired_at timestamptz not null default now(),
    revoked_at timestamptz
);

create table projects (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references auth.users (id) on delete cascade,
    name text not null,
    directory text not null,
    agent text not null default 'opencode',
    priority text not null default 'NORMAL',
    created_at timestamptz not null default now()
);

create table sessions (
    id text primary key,
    project_id uuid references projects (id) on delete cascade,
    project_dir text not null default '',
    status text not null default 'STARTING',
    started_at timestamptz not null default now(),
    ended_at timestamptz
);

create table events (
    id uuid primary key default gen_random_uuid(),
    session_id text not null references sessions (id) on delete cascade,
    type text not null,
    payload jsonb,
    created_at timestamptz not null default now()
);

create table commands (
    id uuid primary key default gen_random_uuid(),
    session_id text not null references sessions (id) on delete cascade,
    type text not null,
    payload jsonb,
    status text not null default 'PENDING',
    created_at timestamptz not null default now()
);

create table notification_preferences (
    user_id uuid primary key references auth.users (id) on delete cascade,
    needs_input boolean not null default true,
    permission_required boolean not null default true,
    completed boolean not null default true,
    crashed boolean not null default true,
    quota_warning boolean not null default true
);

create table pairing_codes (
    id uuid primary key default gen_random_uuid(),
    code text not null unique,
    user_id uuid not null references auth.users (id) on delete cascade,
    expires_at timestamptz not null,
    created_at timestamptz not null default now()
);

alter table devices enable row level security;
alter table projects enable row level security;
alter table sessions enable row level security;
alter table events enable row level security;
alter table commands enable row level security;
alter table notification_preferences enable row level security;
alter table pairing_codes enable row level security;

-- RLS policies. Enabled per PRD section 13. These are permissive for the MVP
-- (authenticated role can read/write all rows) and must be refined with
-- auth.uid() scoping once Supabase auth is integrated. Without policies,
-- RLS with no policy blocks all access by default.
create policy sessions_all on sessions
    for all to authenticated using (true) with check (true);
create policy events_all on events
    for all to authenticated using (true) with check (true);
create policy commands_all on commands
    for all to authenticated using (true) with check (true);
create policy devices_all on devices
    for all to authenticated using (true) with check (true);
create policy projects_all on projects
    for all to authenticated using (true) with check (true);
create policy pairing_codes_all on pairing_codes
    for all to authenticated using (true) with check (true);
create policy notification_preferences_all on notification_preferences
    for all to authenticated using (true) with check (true);
