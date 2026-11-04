-- 0003_mvp_tables.sql
-- MVP tables for tasks, incidents, approvals, recovery_actions, commands.
-- Synced by the agent-manager via the supabase client.
-- Apply after 0001_init.sql and 0002_add_session_columns.sql.
--
-- Notes on idempotency / ordering:
--   * tasks.approval_id is a plain uuid column here. The FK to approvals is
--     added via ALTER TABLE *after* approvals exists (circular dependency).
--   * commands already exists from 0001_init.sql; the CREATE below is a no-op.
--   * PostgreSQL has no CREATE POLICY IF NOT EXISTS; use DROP POLICY IF EXISTS.

-- ---------------------------------------------------------------
-- 1. Tasks table (approval_id has NO inline FK — see step 4)
-- ---------------------------------------------------------------
create table if not exists tasks (
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
    approval_id uuid,
    recovery_count integer not null default 0
);

-- ---------------------------------------------------------------
-- 2. Incidents table
-- ---------------------------------------------------------------
create table if not exists incidents (
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

-- ---------------------------------------------------------------
-- 3. Approvals table
-- ---------------------------------------------------------------
create table if not exists approvals (
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

-- ---------------------------------------------------------------
-- 4. Add the deferred FK tasks -> approvals.
--    Idempotent: only add if it doesn't already exist.
-- ---------------------------------------------------------------
do $$
begin
    if not exists (
        select 1 from pg_constraint where conname = 'tasks_approval_id_fkey'
    ) then
        alter table tasks
            add constraint tasks_approval_id_fkey
            foreign key (approval_id)
            references approvals (id)
            on delete set null;
    end if;
end $$;

-- ---------------------------------------------------------------
-- 5. Recovery actions and commands
-- ---------------------------------------------------------------
create table if not exists recovery_actions (
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

-- commands already exists from 0001_init.sql; this is a no-op on your DB.
-- Kept here so a fresh environment that runs only 0003 still works, but note
-- 0001's definition (no updated_at) wins when both are applied.
create table if not exists commands (
    id uuid primary key default gen_random_uuid(),
    session_id text not null references sessions (id) on delete cascade,
    type text not null,
    payload jsonb,
    status text not null default 'PENDING',
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

-- ---------------------------------------------------------------
-- 6. Enable RLS + create policies (drop-then-create for idempotency)
-- ---------------------------------------------------------------
alter table tasks enable row level security;
alter table incidents enable row level security;
alter table approvals enable row level security;
alter table recovery_actions enable row level security;
alter table commands enable row level security;

drop policy if exists tasks_all on tasks;
create policy tasks_all on tasks
    for all to authenticated using (true) with check (true);

drop policy if exists incidents_all on incidents;
create policy incidents_all on incidents
    for all to authenticated using (true) with check (true);

drop policy if exists approvals_all on approvals;
create policy approvals_all on approvals
    for all to authenticated using (true) with check (true);

drop policy if exists recovery_actions_all on recovery_actions;
create policy recovery_actions_all on recovery_actions
    for all to authenticated using (true) with check (true);

-- commands_all may already exist from 0001; drop-then-create handles both.
drop policy if exists commands_all on commands;
create policy commands_all on commands
    for all to authenticated using (true) with check (true);