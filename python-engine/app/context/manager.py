# Context window management and compression
from __future__ import annotations

import logging
import math
from typing import Optional

logger = logging.getLogger(__name__)

# System message overhead per the OpenAI/Anthropic convention
_SYSTEM_OVERHEAD_TOKENS = 4  # role framing per message


class ContextManager:
    """Manages conversation context windows with automatic compression.

    Handles:
    - Approximate token counting (no tiktoken dependency)
    - Context window management
    - Automatic compression when approaching limits
    - Message history trimming
    """

    def __init__(self, max_tokens: int = 200_000, compression_threshold: float = 0.8):
        """
        Args:
            max_tokens: Maximum context window size in tokens.
            compression_threshold: Ratio of max_tokens at which compression kicks in
                (e.g. 0.8 means compress when usage exceeds 80 %).
        """
        self.max_tokens = max_tokens
        self.compression_threshold = compression_threshold

    # ------------------------------------------------------------------
    # Token counting
    # ------------------------------------------------------------------

    @staticmethod
    def count_tokens(text: str) -> int:
        """Approximate token count for a string.

        Heuristic:
        - English / ASCII: ~4 characters per token
        - CJK / wide characters: ~2 characters per token
        - Minimum 1 token for any non-empty text

        This is intentionally simple — it avoids a hard dependency on tiktoken
        while being accurate enough for budget management.
        """
        if not text:
            return 0

        ascii_chars = 0
        non_ascii_chars = 0

        for ch in text:
            if ord(ch) < 128:
                ascii_chars += 1
            else:
                non_ascii_chars += 1

        # English: ~4 chars/token, CJK/other: ~2 chars/token
        tokens = math.ceil(ascii_chars / 4) + math.ceil(non_ascii_chars / 2)
        return max(tokens, 1) if text.strip() else 0

    def count_message_tokens(self, messages: list) -> int:
        """Count total tokens across a list of message dicts.

        Each message is expected to be a dict with at least 'role' and 'content'.
        A small per-message overhead is added for framing.
        """
        total = 0
        for msg in messages:
            # Per-message framing overhead
            total += _SYSTEM_OVERHEAD_TOKENS

            content = msg.get("content", "")
            if isinstance(content, str):
                total += self.count_tokens(content)
            elif isinstance(content, list):
                # Multi-modal content: count text parts
                for part in content:
                    if isinstance(part, dict) and part.get("type") == "text":
                        total += self.count_tokens(part.get("text", ""))
                    elif isinstance(part, str):
                        total += self.count_tokens(part)

            # Count tool call arguments if present
            tool_calls = msg.get("tool_calls")
            if tool_calls:
                for tc in tool_calls:
                    if isinstance(tc, dict):
                        args = tc.get("arguments", "")
                        if isinstance(args, str):
                            total += self.count_tokens(args)

        return total

    # ------------------------------------------------------------------
    # Compression
    # ------------------------------------------------------------------

    async def compress(self, messages: list, gateway=None) -> list:
        """Compress messages if approaching the token limit.

        Strategy:
        1. If total tokens < compression_threshold * max_tokens, return unchanged.
        2. Keep the system prompt (first system message) and the last N messages intact.
        3. Summarise the middle messages into a single context-summary message.
        4. If *gateway* is provided, use it to generate a real summary; otherwise
           fall back to a simple truncation summary.

        Args:
            messages: List of message dicts (role, content, ...).
            gateway: Optional LLM gateway for generating summaries.

        Returns:
            Compressed message list (may be the same reference if no compression
            was needed).
        """
        total_tokens = self.count_message_tokens(messages)
        threshold_tokens = int(self.max_tokens * self.compression_threshold)

        if total_tokens <= threshold_tokens:
            return messages

        logger.info(
            "Context compression triggered: %d tokens > threshold %d",
            total_tokens,
            threshold_tokens,
        )

        # Split messages into: system | middle (to compress) | tail (to keep)
        system_msgs: list[dict] = []
        other_msgs: list[dict] = []

        for msg in messages:
            if msg.get("role") == "system" and not system_msgs:
                system_msgs.append(msg)
            else:
                other_msgs.append(msg)

        # How many tail messages to preserve (at least 4 for a sensible conversation)
        keep_tail = max(4, len(other_msgs) // 3)

        if len(other_msgs) <= keep_tail:
            # Nothing to compress
            return messages

        middle = other_msgs[:-keep_tail]
        tail = other_msgs[-keep_tail:]

        # Build a summary of the middle portion
        summary_text = await self._summarise(middle, gateway)

        summary_msg = {
            "role": "system",
            "content": (
                "[Context Summary]\n"
                "The following is a compressed summary of earlier conversation messages "
                "that were removed to stay within the context window.\n\n"
                f"{summary_text}"
            ),
        }

        compressed = system_msgs + [summary_msg] + tail

        new_tokens = self.count_message_tokens(compressed)
        logger.info(
            "Compression result: %d → %d tokens (%.1f%% reduction)",
            total_tokens,
            new_tokens,
            (1 - new_tokens / total_tokens) * 100 if total_tokens else 0,
        )

        return compressed

    async def _summarise(self, messages: list, gateway=None) -> str:
        """Produce a text summary of *messages*.

        If *gateway* is supplied and exposes ``chat``, ask the LLM to summarise.
        Otherwise build a lightweight extraction-based summary.
        """
        if gateway is not None:
            try:
                return await self._llm_summarise(messages, gateway)
            except Exception as exc:
                logger.warning("LLM summarisation failed, falling back: %s", exc)

        # Fallback: extract key lines from each message
        parts: list[str] = []
        for msg in messages:
            role = msg.get("role", "user")
            content = msg.get("content", "")
            if isinstance(content, str) and content.strip():
                # Take the first 200 chars of each message as a snippet
                snippet = content[:200].replace("\n", " ")
                parts.append(f"[{role}]: {snippet}")
        return "\n".join(parts) if parts else "(no content)"

    @staticmethod
    async def _llm_summarise(messages: list, gateway) -> str:
        """Use the gateway to produce a real summary."""
        summary_prompt = [
            {
                "role": "system",
                "content": (
                    "You are a context compressor. Summarise the following conversation "
                    "messages concisely, preserving all key facts, decisions, and context "
                    "needed to continue the conversation. Reply with the summary only."
                ),
            },
            {
                "role": "user",
                "content": "\n".join(
                    f"[{m.get('role', 'user')}]: {m.get('content', '')}"
                    for m in messages
                    if isinstance(m.get('content'), str)
                ),
            },
        ]
        # Try the gateway's non-streaming chat interface
        from app.config import settings
        resp = await gateway.chat(messages=summary_prompt, model=settings.default_model, max_tokens=1024)
        return getattr(resp, "content", "") or "(summary generation produced no content)"

    # ------------------------------------------------------------------
    # Trimming
    # ------------------------------------------------------------------

    def trim_to_fit(self, messages: list, max_tokens: int | None = None) -> list:
        """Remove oldest non-system messages until the total fits within *max_tokens*.

        The system prompt (first system message) is always preserved.

        Args:
            messages: List of message dicts.
            max_tokens: Token budget (defaults to self.max_tokens).

        Returns:
            A (possibly shorter) message list whose token count ≤ *max_tokens*.
        """
        budget = max_tokens if max_tokens is not None else self.max_tokens

        if self.count_message_tokens(messages) <= budget:
            return messages

        # Separate the system message(s) from the rest
        system_msgs: list[dict] = []
        other_msgs: list[dict] = []

        for msg in messages:
            if msg.get("role") == "system" and not system_msgs:
                system_msgs.append(msg)
            else:
                other_msgs.append(msg)

        system_tokens = self.count_message_tokens(system_msgs)
        remaining_budget = budget - system_tokens

        # Walk from the end (most recent) backwards, accumulating until we hit the budget
        kept: list[dict] = []
        used = 0

        for msg in reversed(other_msgs):
            msg_tokens = self.count_message_tokens([msg])
            if used + msg_tokens > remaining_budget and kept:
                break
            kept.append(msg)
            used += msg_tokens

        kept.reverse()

        result = system_msgs + kept
        logger.info(
            "Trimmed messages: %d → %d (%d tokens)",
            len(messages),
            len(result),
            self.count_message_tokens(result),
        )
        return result
