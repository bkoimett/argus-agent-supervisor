-- Add columns written by supabase.SyncSession that were missing from the
-- initial sessions schema (caused PGRST204: sessions.agent missing from the
-- schema cache). Column types mirror the Go state.Session fields written by
-- the agent-manager; nothing here is guessed.
--
-- Apply after 0001_init.sql.

alter table sessions
    add column if not exists agent text not null default 'opencode';

alter table sessions
    add column if not exists model text;

alter table sessions
    add column if not exists pid integer;

alter table sessions
    add column if not exists error_message text;

alter table sessions
    add column if not exists updated_at timestamptz not null default now();

-- Keep updated_at fresh on future writes. Supabase does not auto-update
-- timestamps unless a trigger/function is defined; the agent-manager sets
-- updated_at explicitly on every SyncSession call.
