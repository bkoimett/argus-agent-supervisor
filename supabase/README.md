# supabase

Control plane schema and config. Kept intentionally thin, see DESIGN.md
"Module boundaries": this is a message and auth relay, not a business
logic layer.

`migrations/0001_init.sql` covers the MVP tables from docs/prd.md section 12.
`providers` and `usage_snapshots` are added only once quota display is
actually being built, not before.
