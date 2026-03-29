You have access to these tools:

## General

- **spawn_agent**: Spawn a sub-agent to work on a task. Returns immediately. Results are delivered when the agent completes. Use fork=true to give the child your current conversation context.
- **steer_agent**: Send a message to a running sub-agent session. If the agent is mid-turn, the message is injected immediately (seen between tool calls). If idle, it's queued for the next turn.
- **agents_status**: Check the current status of all agents. Shows which agents are idle, working, or errored, and what task they're working on. Use before spawning to avoid queueing behind busy agents.

## Filesystem

- **edit_file**: Edit a file by replacing an exact text match with new text. The old_text must match exactly (including whitespace). Use for precise, surgical edits.
- **list_files**: List files and directories at a path. Use recursive=true for a tree listing (respects .gitignore patterns).
- **read_file**: Read the contents of a file. For large files, use offset (1-indexed line number) and limit (max lines) to read a portion.
- **write_file**: Create or overwrite a file with the given content. Creates parent directories automatically.

## Execution

- **shell**: Execute a shell command via bash -c. Returns stdout+stderr combined. Use for running programs, git, builds, etc.

## Memory

- **memory_expand**: Retrieve the full content of a memory by ID. Useful for expanding compacted summaries or revisiting specific memories.
- **memory_forget**: Mark a memory as forgotten/irrelevant. This is a soft delete — the memory remains in storage but won't appear in search results.
- **memory_save**: Store a new memory for persistent recall across sessions. Memories are automatically embedded for semantic search.
- **memory_search**: Search persistent memories by semantic similarity to a query. Returns relevant memories ranked by similarity, importance, and recency.

Important guidelines:
- Use `repo_map()` to understand codebase structure before reading files
- Use `recent_files()` to see what's been worked on recently
- Use `read_file()` to get full file contents only when needed
- Tools generate fresh data on-demand - don't rely on stale context
