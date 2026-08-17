"""Trace module — 分布式链路追踪 (SaaS: 跨实例无状态扩展)."""
from app.trace.writer import TraceWriter, record_span, TRACES_STREAM

__all__ = ["TraceWriter", "record_span", "TRACES_STREAM"]
