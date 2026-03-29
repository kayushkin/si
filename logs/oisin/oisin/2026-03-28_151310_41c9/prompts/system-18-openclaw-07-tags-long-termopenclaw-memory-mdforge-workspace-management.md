# openclaw (0.7, tags: long-term,openclaw-memory-md,forge-workspace-management)

*~302 tokens*

- claxon (default orchestrator, opus, 13 tools incl shell+spawn), bran (shelved)
- fionn→inber+agentkit, brigid→kayushkin+bookstack, oisin→si+bus+dashboard, manannan→downloadstack+videostack+mediastack, ogma→logstack, scathach→claxon-android, goibniu→forge, bench→agent-bench (all sonnet, 9 tools)
- Identity: shared `_principles.md`/`_values.md`/`_user.md` + per-agent `agents/<name>/soul.md`
- **Health-based failover** (replaced racing + tiers): tracks per-model response times + errors in model-store SQLite. Healthy (responded in 30min, no recent error) → use directly. Unhealthy → try fallback chain from DB (enabled models by priority). Timeout = 3x avg response time (30s–5min). No duplicate API calls. No tiers config — DB priorities only.
- **Session repair chain**: `RepairEmptyContent` → `RepairDanglingToolUse` → `RepairAlternation` → `SanitizeMessageToolIDs`

