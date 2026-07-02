"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { wsUrl } from "@/lib/api";

type ConnectionStatus = "connecting" | "connected" | "disconnected";
type MessageHandler = (data: Record<string, unknown>) => void;

interface UseWebSocketOptions {
  sessionId: string;
  onMessage?: MessageHandler;
}

export function useWebSocket({ sessionId, onMessage }: UseWebSocketOptions) {
  const [status, setStatus] = useState<ConnectionStatus>("disconnected");
  const wsRef = useRef<WebSocket | null>(null);
  const onMessageRef = useRef<MessageHandler | undefined>(onMessage);
  onMessageRef.current = onMessage;

  const connect = useCallback(() => {
    if (wsRef.current?.readyState === WebSocket.OPEN) return;

    setStatus("connecting");
    const url = wsUrl(`/ws/${sessionId}`);
    const ws = new WebSocket(url);

    ws.onopen = () => setStatus("connected");
    ws.onclose = () => {
      setStatus("disconnected");
      wsRef.current = null;
    };
    ws.onerror = () => {
      setStatus("disconnected");
    };
    ws.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data) as Record<string, unknown>;
        onMessageRef.current?.(data);
      } catch {
        // ignore non-JSON messages
      }
    };

    wsRef.current = ws;
  }, [sessionId]);

  const disconnect = useCallback(() => {
    wsRef.current?.close();
    wsRef.current = null;
    setStatus("disconnected");
  }, []);

  const send = useCallback((data: Record<string, unknown>) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify(data));
    }
  }, []);

  useEffect(() => {
    return () => {
      wsRef.current?.close();
    };
  }, []);

  return { status, connect, disconnect, send };
}
