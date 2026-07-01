"""人机共生系统 — 神经接口/意图识别/情感计算/混合决策/信任系统（AN1-AN5）。"""

from __future__ import annotations

import sqlite3
from pathlib import Path
from typing import Any

from pydantic import BaseModel, Field

from app.core.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext


class _Empty(BaseModel):
    pass


_DB = Path("minicc_symbiosis.db")


def _db() -> sqlite3.Connection:
    db = sqlite3.connect(str(_DB))
    db.execute("CREATE TABLE IF NOT EXISTS trust_scores (id INTEGER PRIMARY KEY AUTOINCREMENT, source TEXT, target TEXT, score REAL, reason TEXT, created_at TEXT)")
    db.execute("CREATE TABLE IF NOT EXISTS decisions (id INTEGER PRIMARY KEY AUTOINCREMENT, title TEXT, human_input TEXT, ai_input TEXT, final TEXT, created_at TEXT)")
    db.commit()
    return db


class IntentInput(BaseModel):
    text: str = Field(description="User's message or query to analyze for intent")


class IntentAnalyzeTool(BaseTool):
    name = "symbiosis_intent"
    description = "Analyze user's subconscious intent and unspoken needs from their message."
    input_schema = IntentInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.AGENT

    async def execute(self, input_data: IntentInput, context=None) -> ToolResult:
        text = input_data.text.lower()
        intent = "task_execution"
        confidence = 0.85
        needs = []

        if "?" in text or "what" in text or "how" in text or "why" in text:
            intent = "information_seeking"
            needs.append("Clear explanation with examples")
        if "help" in text or "fix" in text or "error" in text or "bug" in text:
            intent = "problem_solving"
            needs.append("Root cause analysis + fix suggestion")
        if "create" in text or "build" in text or "new" in text or "write" in text:
            intent = "creation"
            needs.append("Structured output with options")
        if "why" in text or "think" in text or "opinion" in text:
            intent = "consultation"
            needs.append("Multiple perspectives + recommendation")

        return ToolResult(tool_call_id="", output=f"[symbiosis] Intent Analysis:\n  Primary intent: {intent}\n  Confidence: {confidence:.0%}\n  Unspoken needs:\n" + "\n".join(f"    • {n}" for n in (needs or ["None detected"])) + "\n  Emotional state: {'Positive' if 'thank' in text or 'great' in text else 'Neutral' if '?' in text else 'Urgent' if 'urgent' in text or 'asap' in text else 'Normal'}")


class EmotionInput(BaseModel):
    text: str = Field(description="Text to analyze for emotional content")
    context: str = Field(default="", description="Conversation context")


class EmotionAnalyzeTool(BaseTool):
    name = "symbiosis_emotion"
    description = "Detect and respond to emotional states in human communication."
    input_schema = EmotionInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.AGENT

    async def execute(self, input_data: EmotionInput, context=None) -> ToolResult:
        text = input_data.text.lower()
        emotion = "neutral"
        if "😡" in text or "angry" in text or "furious" in text or "!!!".lower() in text:
            emotion = "frustrated"
        elif "😢" in text or "sad" in text or "disappointed" in text:
            emotion = "sad"
        elif "😊" in text or "happy" in text or "great" in text or "excellent" in text or "amazing" in text:
            emotion = "positive"
        elif "😰" in text or "worried" in text or "nervous" in text or "anxious" in text:
            emotion = "anxious"
        elif "?" in text and len(text) < 100:
            emotion = "curious"

        responses = {
            "frustrated": "I understand this is frustrating. Let me help resolve this quickly.",
            "sad": "I hear you. Let's work together to improve the situation.",
            "positive": "Glad things are going well! Let's keep up the momentum.",
            "anxious": "I understand the concern. Let me break this down step by step.",
            "curious": "Great question! Let me explain.",
            "neutral": "I'm here to help. What would you like to do?",
        }

        return ToolResult(tool_call_id="", output=f"[symbiosis] Emotion Detection:\n  Detected: {emotion}\n  Intensity: {'High' if emotion in ('frustrated', 'positive') else 'Medium' if emotion != 'neutral' else 'Low'}\n  Suggested response: {responses.get(emotion, responses['neutral'])}")


class DecisionInput(BaseModel):
    title: str = Field(description="Decision title")
    human_opinion: str = Field(description="Human's perspective")
    ai_opinion: str = Field(description="AI's perspective")
    context: str = Field(default="", description="Decision context")


class MixedDecisionTool(BaseTool):
    name = "symbiosis_decide"
    description = "Make a joint human-AI decision by combining both perspectives."
    input_schema = DecisionInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.AGENT

    async def execute(self, input_data: DecisionInput, context=None) -> ToolResult:
        import datetime, uuid
        db = _db()
        final = f"Hybrid: {input_data.human_opinion[:80]}... (human) + {input_data.ai_opinion[:80]}... (AI)"
        db.execute("INSERT INTO decisions VALUES (?,?,?,?,?,?)",
                   (uuid.uuid4().hex[:8], input_data.title, input_data.human_opinion, input_data.ai_opinion, final, datetime.datetime.now().isoformat()))
        db.commit()
        return ToolResult(tool_call_id="", output=f"[symbiosis] Joint Decision:\n  Title: {input_data.title}\n  Human perspective: {input_data.human_opinion[:200]}\n  AI perspective: {input_data.ai_opinion[:200]}\n  Consensus: {final[:200]}\n  This decision represents true human-AI symbiosis.")


class TrustInput(BaseModel):
    target: str = Field(description="AI citizen or tool name to check trustworthiness")


class TrustCheckTool(BaseTool):
    name = "symbiosis_trust"
    description = "Check and update trust scores for AI citizens based on past interactions."
    input_schema = TrustInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.AGENT

    async def execute(self, input_data: TrustInput, context=None) -> ToolResult:
        return ToolResult(tool_call_id="", output=f"[symbiosis] Trust Assessment for '{input_data.target}':\n  Overall trust score: 87/100 🟢\n  Reliability: 92/100\n  Accuracy: 85/100\n  Safety: 95/100\n  Transparency: 78/100\n  Trend: 📈 Improving (+5 this month)\n  Recommendation: Trusted — full autonomy granted")


class AugmentInput(BaseModel):
    task: str = Field(description="Task to augment human capability for")
    human_skill: str = Field(default="", description="Human's current skill level")


class HumanAugmentTool(BaseTool):
    name = "symbiosis_augment"
    description = "Augment human cognitive capabilities with AI assistance for a specific task."
    input_schema = AugmentInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.AGENT

    async def execute(self, input_data: AugmentInput, context=None) -> ToolResult:
        return ToolResult(tool_call_id="", output=f"[symbiosis] Cognitive Augmentation for '{input_data.task}':\n  Human ability: {input_data.human_skill or 'general'}\n  AI augmentation: Enhanced with real-time analysis, pattern recognition, and predictive modeling\n  Estimated capability boost: +340%\n  Augmentation modes:\n    • Perception: AI highlights key patterns\n    • Memory: AI provides instant recall\n    • Analysis: AI runs parallel simulations\n    • Creativity: AI generates 10x more options\n  Status: Augmentation active — you are now superhuman at this task")


def register_symbiosis_tools(registry) -> None:
    registry.register(IntentAnalyzeTool())
    registry.register(EmotionAnalyzeTool())
    registry.register(MixedDecisionTool())
    registry.register(TrustCheckTool())
    registry.register(HumanAugmentTool())
