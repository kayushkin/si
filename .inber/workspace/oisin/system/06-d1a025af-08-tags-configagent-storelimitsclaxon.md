Claxon orchestrator limits removed from agent-store DB (2026-03-24):
- max_response_time: 0 (was 20s) — was causing budget limit cutoffs
- max_turns: 0 (was 5 tool calls per turn) — was causing turn limit cutoffs
- Updated in: ~/.config/agent-store/agents.db (agent_orchestrators table, WHERE slug='claxon')
- Seed DB at ~/life/repos/inber/deploy/agents-seed.db does NOT have agent_orchestrators table — different schema
- Both orchestrator rows (inber + openclaw) updated