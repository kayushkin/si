# Tools

12 tools available

## 1. edit_file
Edit a file by replacing an exact text match with new text. The old_text must match exactly (including whitespace). Use for precise, surgical edits.

## 2. list_files
List files and directories at a path. Use recursive=true for a tree listing (respects .gitignore patterns).

## 3. read_file
Read the contents of a file. For large files, use offset (1-indexed line number) and limit (max lines) to read a portion.

## 4. shell
Execute a shell command via bash -c. Returns stdout+stderr combined. Use for running programs, git, builds, etc.

## 5. write_file
Create or overwrite a file with the given content. Creates parent directories automatically.

## 6. memory_expand
Retrieve the full content of a memory by ID. Useful for expanding compacted summaries or revisiting specific memories.

## 7. memory_forget
Mark a memory as forgotten/irrelevant. This is a soft delete — the memory remains in storage but won't appear in search results.

## 8. memory_save
Store a new memory for persistent recall across sessions. Memories are automatically embedded for semantic search.

## 9. memory_search
Search persistent memories by semantic similarity to a query. Returns relevant memories ranked by similarity, importance, and recency.

## 10. spawn_agent
Spawn a sub-agent to work on a task. Returns immediately. Results are delivered when the agent completes. Use fork=true to give the child your current conversation context.

## 11. steer_agent
Send a message to a running sub-agent session. If the agent is mid-turn, the message is injected immediately (seen between tool calls). If idle, it's queued for the next turn.

## 12. agents_status
Check the current status of all agents. Shows which agents are idle, working, or errored, and what task they're working on. Use before spawning to avoid queueing behind busy agents.

