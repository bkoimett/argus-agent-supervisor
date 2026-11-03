-- MVP tables for tasks, incidents, approvals, recovery_actions, commands.
-- These are synced by the agent-manager via supabase client.
-- Apply after 0001_init.sql and 0002_add_session_columns.sql.

-- Tasks table
create table tasks (
    id uuid primary key default gen_random_uuid(),
    session_id text not null references sessions (id) on delete cascade,
    prompt text not null,
    scope text,
    dependencies jsonb,
    status text not null default 'PENDING',
    started_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    ended_at timestamptz,
    exit_code integer,
    error_message text,
    approval_id uuid references approvals (id) on delete set null,
    recovery_count integer not null default 0
);

-- Incidents table
create table incidents (
    id uuid primary key default gen_random_uuid(),
    session_id text not null references sessions (id) on delete cascade,
    task_id uuid references tasks (id) on delete set null,
    type text not null,
    severity text not null,
    status text not null default 'DETECTED',
    evidence jsonb,
    meta_diagnosis text,
    meta_recovery jsonb,
    detected_at timestamptz not null default now(),
    diagnosed_at timestamptz,
    proposed_at timestamptz,
    approved_at timestamptz,
    recovered_at timestamptz,
    verified_at timestamptz,
    recovery_attempts integer not null default 0,
    updated_at timestamptz not null default now()
);

-- Approvals table
create table approvals (
    id uuid primary key default gen_random_uuid(),
    session_id text not null references sessions (id) on delete cascade,
    task_id uuid references tasks (id) on delete set null,
    incident_id uuid references incidents (id) on delete set null,
    type text not null,
    status text not null default 'PENDING',
    title text not null,
    description text,
    payload jsonb,
    scope text,
    requested_at timestamptz not null default now(),
    expires_at timestamptz not null,
    responded_at timestamptz,
    decision boolean,
    decided_by text,
    updated_at timestamptz not null default now()
);

-- Recovery actions table
create table recovery_actions (
    id uuid primary key default gen_random_uuid(),
    incident_id uuid not null references incidents (id) on delete cascade,
    type text not null,
    description text not null,
    command text not null,
    args jsonb,
    status text not null default 'PROPOSED',
    proposed_at timestamptz not null default now(),
    approved_at timestamptz,
    executed_at timestamptz,
    result jsonb,
    updated_at timestamptz not null default now()
);

-- Commands table (for PWA -> agent messaging)
create table commands (
    id uuid primary key default gen_random_uuid(),
    session_id text not null references sessions (id) on delete cascade,
    type text not null,
    payload jsonb,
    status text not null default 'PENDING',
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

-- Enable RLS on all new tables
alter table tasks enable row level security;
alter table incidents enable row level security;
alter table approvals enable row level security;
alter table recovery_actions enable row level security;
alter table commands enable row level security;

-- Permissive RLS policies for MVP (authenticated role can read/write all rows)
-- TODO: Scope to auth.uid() once Supabase auth is integrated.
create policy tasks_all on tasks
    for all to authenticated using (true) with check (true);
create policy incidents_all on incidents
    for all to authenticated using (true) with check (true);
create policy approvals_all on approvals
    for all to authenticated using (true) with check (true);
create policy recovery_actions_all on recovery_actions
    for all to authenticated using (true) with check (true);
create policy commands_all on commands
    for all to authenticated using (true) with check (true);