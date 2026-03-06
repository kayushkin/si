# Tools

11 tools available

## 1. shell
Execute a shell command via bash -c. Returns stdout+stderr combined. Use for running programs, git, builds, etc.

## 2. read_file
Read the contents of a file. For large files, use offset (1-indexed line number) and limit (max lines) to read a portion.

## 3. write_file
Create or overwrite a file with the given content. Creates parent directories automatically.

## 4. edit_file
Edit a file by replacing an exact text match with new text. The old_text must match exactly (including whitespace). Use for precise, surgical edits.

## 5. list_files
List files and directories at a path. Use recursive=true for a tree listing (respects .gitignore patterns).

## 6. memory_search
Search persistent memories by semantic similarity to a query. Returns relevant memories ranked by similarity, importance, and recency.

## 7. memory_save
Store a new memory for persistent recall across sessions. Memories are automatically embedded for semantic search.

## 8. memory_expand
Retrieve the full content of a memory by ID. Useful for expanding compacted summaries or revisiting specific memories.

## 9. memory_forget
Mark a memory as forgotten/irrelevant. This is a soft delete — the memory remains in storage but won't appear in search results.

## 10. repo_map
Generate a structural map of the codebase showing packages, functions, types, and file organization. Use this to understand the project structure without reading full files.

## 11. recent_files
List files that were recently modified, with metadata like line count, modification time, and importance score. Use this to see what's been actively worked on.

