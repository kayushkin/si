# Tool Definitions

12 tools registered

## 1. edit_file

Edit a file by replacing an exact text match with new text. The old_text must match exactly (including whitespace). Use for precise, surgical edits.

**Schema:**

```json
{
  "properties": {
    "new_text": {
      "description": "New text to replace the old text with",
      "type": "string"
    },
    "old_text": {
      "description": "Exact text to find and replace",
      "type": "string"
    },
    "path": {
      "description": "Path to the file to edit",
      "type": "string"
    }
  },
  "required": [
    "path",
    "old_text",
    "new_text"
  ],
  "type": "object"
}
```

## 2. list_files

List files and directories at a path. Use recursive=true for a tree listing (respects .gitignore patterns).

**Schema:**

```json
{
  "properties": {
    "path": {
      "description": "Directory path to list",
      "type": "string"
    },
    "recursive": {
      "description": "List recursively (default: false)",
      "type": "boolean"
    }
  },
  "required": [
    "path"
  ],
  "type": "object"
}
```

## 3. read_file

Read the contents of a file. For large files, use offset (1-indexed line number) and limit (max lines) to read a portion.

**Schema:**

```json
{
  "properties": {
    "limit": {
      "description": "Maximum number of lines to return (optional)",
      "type": "integer"
    },
    "offset": {
      "description": "Line number to start from (1-indexed, optional)",
      "type": "integer"
    },
    "path": {
      "description": "Path to the file to read",
      "type": "string"
    }
  },
  "required": [
    "path"
  ],
  "type": "object"
}
```

## 4. shell

Execute a shell command via bash -c. Returns stdout+stderr combined. Use for running programs, git, builds, etc.

**Schema:**

```json
{
  "properties": {
    "command": {
      "description": "Shell command to execute",
      "type": "string"
    },
    "workdir": {
      "description": "Working directory (optional, defaults to cwd)",
      "type": "string"
    }
  },
  "required": [
    "command"
  ],
  "type": "object"
}
```

## 5. write_file

Create or overwrite a file with the given content. Creates parent directories automatically.

**Schema:**

```json
{
  "properties": {
    "content": {
      "description": "Content to write to the file",
      "type": "string"
    },
    "path": {
      "description": "Path to the file to write",
      "type": "string"
    }
  },
  "required": [
    "path",
    "content"
  ],
  "type": "object"
}
```

## 6. memory_expand

Retrieve the full content of a memory by ID. Useful for expanding compacted summaries or revisiting specific memories.

**Schema:**

```json
{
  "properties": {
    "id": {
      "description": "Memory ID to retrieve",
      "type": "string"
    }
  },
  "required": [
    "id"
  ],
  "type": "object"
}
```

## 7. memory_forget

Mark a memory as forgotten/irrelevant. This is a soft delete — the memory remains in storage but won't appear in search results.

**Schema:**

```json
{
  "properties": {
    "id": {
      "description": "Memory ID to forget",
      "type": "string"
    }
  },
  "required": [
    "id"
  ],
  "type": "object"
}
```

## 8. memory_save

Store a new memory for persistent recall across sessions. Memories are automatically embedded for semantic search.

**Schema:**

```json
{
  "properties": {
    "content": {
      "description": "The memory content to store",
      "type": "string"
    },
    "importance": {
      "description": "Importance score 0-1 (default: 0.5). Higher scores = higher priority in search.",
      "type": "number"
    },
    "source": {
      "description": "Source of the memory: 'user', 'agent', 'system' (default: 'agent')",
      "type": "string"
    },
    "tags": {
      "description": "Tags for categorization (e.g., 'code', 'preference', 'fact')",
      "items": {
        "type": "string"
      },
      "type": "array"
    }
  },
  "required": [
    "content"
  ],
  "type": "object"
}
```

## 9. memory_search

Search persistent memories by semantic similarity to a query. Returns relevant memories ranked by similarity, importance, and recency.

**Schema:**

```json
{
  "properties": {
    "limit": {
      "description": "Maximum number of results to return (default: 10)",
      "type": "integer"
    },
    "query": {
      "description": "Search query text",
      "type": "string"
    }
  },
  "required": [
    "query"
  ],
  "type": "object"
}
```

## 10. spawn_agent

Spawn a sub-agent to work on a task. Returns immediately. Results are delivered when the agent completes. Use fork=true to give the child your current conversation context.

**Schema:**

```json
{
  "properties": {
    "agent": {
      "description": "Agent name to spawn. Available: [manannan healthcheck bench brigid inber-party etain oisin ogma keyboard scathach argraphments bran fionn goibniu bile dagda lugh claxon]",
      "type": "string"
    },
    "fork": {
      "description": "If true, child inherits this session's conversation history",
      "type": "boolean"
    },
    "model": {
      "description": "Model override (optional)",
      "type": "string"
    },
    "task": {
      "description": "Task for the agent to complete",
      "type": "string"
    },
    "timeout_seconds": {
      "description": "Max runtime in seconds (default 300)",
      "type": "integer"
    }
  },
  "required": [
    "agent",
    "task"
  ],
  "type": "object"
}
```

## 11. steer_agent

Send a message to a running sub-agent session. If the agent is mid-turn, the message is injected immediately (seen between tool calls). If idle, it's queued for the next turn.

**Schema:**

```json
{
  "properties": {
    "message": {
      "description": "Message to send to the agent",
      "type": "string"
    },
    "session_key": {
      "description": "Session key of the target agent (from spawn response or sessions_list)",
      "type": "string"
    }
  },
  "required": [
    "session_key",
    "message"
  ],
  "type": "object"
}
```

## 12. agents_status

Check the current status of all agents. Shows which agents are idle, working, or errored, and what task they're working on. Use before spawning to avoid queueing behind busy agents.

**Schema:**

```json
{
  "properties": {
    "agent_slug": {
      "description": "Optional: check a specific agent by slug",
      "type": "string"
    }
  },
  "type": "object"
}
```

---
**Total:** 12 tools, ~2183 tokens
