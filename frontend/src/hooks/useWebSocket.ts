"use client";

import { useCallback, useEffect, useRef, useState } from "react";

const WS_BASE = "ws://localhost:8080";

type ConnectionStatus = "connecting" | "connected" | "disconnected";
type MessageHandler = (data: Record<string, unknown>) => void;

interface UseWebSocketOptions {
  sessionId: string;
  onMessage?: MessageHandler;
  onStatusChange?: (status: ConnectionStatus) => void;
}

export function useWebSocket({ sessionId, onMessage, onStatusChange }: UseWebSocketOptions) {
  const [status, setStatus] = useState<ConnectionStatus>("disconnected");
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectCount = useRef(0);
  const handlersRef = useRef<Map<string, MessageHandler[]>>(new Map());

  const connect = useCallback(() => {
    const url = `ws://localhost:8080/ws/${sessionId}`;
    setStatus("connecting");
    onStatusChange?.("connecting");

    const ws = new WebSocket(url);
    wsRef.current = ws;

    ws.onopen = () => {
      setStatus("connected");
      onStatusChange?.("connected");
      reconnectCount.current = 0;
    };

    ws.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        onMessage?.(data);

        // 按 type 分发到注册的处理器
        const handlers = handlersRef.current.get(data.type);
        handlers?.forEach((h) => h(data));
      } catch {
        // ignore parse errors
      }
    };

    ws.onclose = () => {
      setStatus("disconnected");
      onStatusChange?.("disconnected");
      scheduleReconnect();
    };

    ws.onerror = () => {
      ws.close();
    };
  }, [sessionId, onMessage, onStatusChange]);

  const scheduleReconnect = useCallback(() => {
    const delay = Math.min(1000 * 2 ** reconnectCount.current, 30000);
    reconnectCount.current += 1;
    setTimeout(connect, delay);
  }, [connect]);

  useEffect(() => {
    connect();
    return () => {
      wsRef.current?.close();
    };
  }, [connect]);

  const send = useCallback((data: string) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(data);
    }
  }, []);

  const sendJSON = useCallback((data: Record<string, unknown>) => {
    send(JSON.stringify(data));
  }, [send]);

  const on = useCallback((type: string, handler: MessageHandler) => {
    const handlers = handlersRef.current.get(type) ?? [];
    handlers.push(handler);
    handlersRef.current.set(type, handlers);
    return () => {
      const remaining = handlers.filter((h) => h !== handler);
      handlersRef.current.set(type, remaining);
    };
  }, []);

  return { status, send, sendJSON, on };
}
