create table agent_runtime_events (
  id bigserial primary key,
  session_id text not null,
  event_offset bigint not null,
  kind text not null default 'pi',
  payload jsonb not null,
  created_at timestamptz not null default now(),
  unique (session_id, event_offset)
);

create index agent_runtime_events_session_created_at_idx
  on agent_runtime_events(session_id, created_at);

create index agent_runtime_events_session_offset_idx
  on agent_runtime_events(session_id, event_offset);
