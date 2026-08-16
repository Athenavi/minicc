"""Built-in slash commands for the Python agent engine.

Every handler has the signature ``async def handler(args: str, ctx: CommandContext) -> str``.
"""
from __future__ import annotations

import json
from typing import Any

from app.commands.registry import CommandContext, registry


# ── /help ───────────────────────────────────────────────────────

async def _help(args: str, ctx: CommandContext | None) -> str:
    """List all available slash commands."""
    cmds = registry.list_commands()
    lines = ["Available commands:\n"]
    for cmd in cmds:
        lines.append(f"  /{cmd.name:<20s} {cmd.description}")
    return "\n".join(lines)


# ── /clear ──────────────────────────────────────────────────────

async def _clear(args: str, ctx: CommandContext | None) -> str:
    """Clear conversation history."""
    if ctx is not None:
        ctx.history.clear()
    return "Conversation history cleared."


# ── /compact ────────────────────────────────────────────────────

async def _compact(args: str, ctx: CommandContext | None) -> str:
    """Compress / summarise conversation context."""
    if ctx is None or not ctx.history:
        return "No conversation to compact."
    count = len(ctx.history)
    summary = f"Compacted {count} messages into context summary."
    if ctx is not None:
        ctx.history.clear()
        ctx.history.append({
            "role": "system",
            "content": f"[Compacted summary of {count} previous messages]",
        })
    return summary


# ── /model <name> ───────────────────────────────────────────────

async def _model(args: str, ctx: CommandContext | None) -> str:
    """Switch the LLM model."""
    if not args.strip():
        current = ctx.model if ctx else "unknown"
        return f"Current model: {current}. Usage: /model <name>"
    if ctx is not None:
        ctx.model = args.strip()
    return f"Model switched to: {args.strip()}"


# ── /temperature <value> ────────────────────────────────────────

async def _temperature(args: str, ctx: CommandContext | None) -> str:
    """Adjust the sampling temperature."""
    if not args.strip():
        current = ctx.temperature if ctx else "unknown"
        return f"Current temperature: {current}. Usage: /temperature <0.0-2.0>"
    try:
        value = float(args.strip())
    except ValueError:
        return f"Invalid temperature: {args.strip()}. Must be a number."
    if not 0.0 <= value <= 2.0:
        return f"Temperature must be between 0.0 and 2.0, got {value}."
    if ctx is not None:
        ctx.temperature = value
    return f"Temperature set to {value}."


# ── /think ──────────────────────────────────────────────────────

async def _think(args: str, ctx: CommandContext | None) -> str:
    """Enter thinking / plan mode."""
    if ctx is not None:
        ctx.metadata["mode"] = "think"
    return "Entered thinking mode. The agent will plan before acting."


# ── /act ────────────────────────────────────────────────────────

async def _act(args: str, ctx: CommandContext | None) -> str:
    """Exit thinking mode and start acting."""
    if ctx is not None:
        ctx.metadata["mode"] = "act"
    return "Exited thinking mode. The agent will now take actions."


# ── /skill <name> ───────────────────────────────────────────────

async def _skill(args: str, ctx: CommandContext | None) -> str:
    """Load a skill by name."""
    name = args.strip()
    if not name:
        return "Usage: /skill <name>"
    if ctx is not None:
        ctx.metadata["active_skill"] = name
    return f"Skill '{name}' loaded."


# ── /memory <query> ─────────────────────────────────────────────

async def _memory(args: str, ctx: CommandContext | None) -> str:
    """Search memories."""
    query = args.strip()
    if not query:
        return "Usage: /memory <query>"
    # Real implementation would talk to the memory manager.
    # Placeholder for built-in command registration.
    return f"Memory search for: '{query}' (no memory backend attached)."


# ── /context ────────────────────────────────────────────────────

async def _context(args: str, ctx: CommandContext | None) -> str:
    """Show current context information."""
    parts = ["Current context:"]
    if ctx is not None:
        parts.append(f"  Model:       {ctx.model or '(not set)'}")
        parts.append(f"  Temperature: {ctx.temperature}")
        parts.append(f"  Mode:        {ctx.metadata.get('mode', 'act')}")
        parts.append(f"  Skill:       {ctx.metadata.get('active_skill', '(none)')}")
        parts.append(f"  History:     {len(ctx.history)} messages")
    else:
        parts.append("  (no context available)")
    return "\n".join(parts)


# ── /cost ───────────────────────────────────────────────────────

async def _cost(args: str, ctx: CommandContext | None) -> str:
    """Show token usage and estimated cost."""
    total = sum(len(m.get("content", "")) for m in (ctx.history if ctx else []))
    parts = [
        "Token usage (approximate):",
        f"  Total characters in history: {total}",
        "  (Detailed token counting requires the LLM gateway.)",
    ]
    return "\n".join(parts)


# ── /undo ───────────────────────────────────────────────────────

async def _undo(args: str, ctx: CommandContext | None) -> str:
    """Undo last file edit."""
    if ctx is not None:
        last_edit = ctx.metadata.pop("last_edit", None)
        if last_edit:
            return f"Undone: {last_edit}"
    return "Nothing to undo."


# ── /retry ──────────────────────────────────────────────────────

async def _retry(args: str, ctx: CommandContext | None) -> str:
    """Retry the last agent response."""
    if ctx is not None:
        ctx.metadata["retry"] = True
    return "Retrying last agent response..."


# ── register all built-ins ──────────────────────────────────────

_BUILTINS: list[tuple[str, str, Any]] = [
    ("help",        "List all available commands",             _help),
    ("clear",       "Clear conversation history",             _clear),
    ("compact",     "Compress/summarize conversation context", _compact),
    ("model",       "Switch LLM model (/model <name>)",       _model),
    ("temperature", "Adjust temperature (/temperature <val>)", _temperature),
    ("think",       "Enter thinking/plan mode",                _think),
    ("act",         "Exit thinking mode, start acting",        _act),
    ("skill",       "Load a skill (/skill <name>)",            _skill),
    ("memory",      "Search memories (/memory <query>)",       _memory),
    ("context",     "Show current context",                    _context),
    ("cost",        "Show token usage and cost",               _cost),
    ("undo",        "Undo last file edit",                     _undo),
    ("retry",       "Retry last agent response",               _retry),
]


def register_builtins() -> None:
    """Register all built-in commands into the global registry."""
    for name, desc, handler in _BUILTINS:
        registry.register(name, desc, handler)
