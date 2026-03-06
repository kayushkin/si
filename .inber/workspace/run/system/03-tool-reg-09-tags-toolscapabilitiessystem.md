You have access to these tools:

## Code-Introspection

- **repo_map**: Generate a structural map of the codebase showing packages, functions, types, and file organization. Use this to understand the project structure without reading full files.
- **recent_files**: List files that were recently modified, with metadata like line count, modification time, and importance score. Use this to see what's been actively worked on.

## Execution

- **shell**: Execute a shell command via bash -c. Returns stdout+stderr combined. Use for running programs, git, builds, etc.

## Filesystem

- **read_file**: Read the contents of a file. For large files, use offset (1-indexed line number) and limit (max lines) to read a portion.
- **write_file**: Create or overwrite a file with the given content. Creates parent directories automatically.
- **edit_file**: Edit a file by replacing an exact text match with new text. The old_text must match exactly (including whitespace). Use for precise, surgical edits.
- **list_files**: List files and directories at a path. Use recursive=true for a tree listing (respects .gitignore patterns).

## Memory

- **memory_search**: Search persistent memories by semantic similarity to a query. Returns relevant memories ranked by similarity, importance, and recency.
- **memory_save**: Store a new memory for persistent recall across sessions. Memories are automatically embedded for semantic search.
- **memory_expand**: Retrieve the full content of a memory by ID. Useful for expanding compacted summaries or revisiting specific memories.
- **memory_forget**: Mark a memory as forgotten/irrelevant. This is a soft delete — the memory remains in storage but won't appear in search results.

Important guidelines:
- Use `repo_map()` to understand codebase structure before reading files
- Use `recent_files()` to see what's been worked on recently
- Use `read_file()` to get full file contents only when needed
- Tools generate fresh data on-demand - don't rely on stale context
