"use client";

import { useState, useCallback, useEffect } from "react";
import type { editor } from "monaco-editor";

/**
 * Natural Edit Hook — 选中代码后按 Ctrl+I 触发。
 */
export function useNaturalEdit(editor: editor.IStandaloneCodeEditor | null) {
  const [isOpen, setIsOpen] = useState(false);
  const [selectedText, setSelectedText] = useState("");
  const [position, setPosition] = useState({ x: 0, y: 0 });

  const openNaturalEdit = useCallback(() => {
    if (!editor) return;
    const selection = editor.getSelection();
    if (!selection || selection.isEmpty()) return;

    const text = editor.getModel()?.getValueInRange(selection) || "";
    setSelectedText(text);

    const pos = editor.getScrolledVisiblePosition(selection.getPosition());
    if (pos) {
      setPosition({ x: pos.left, y: pos.top + pos.height + 10 });
    }
    setIsOpen(true);
  }, [editor]);

  const closeNaturalEdit = useCallback(() => {
    setIsOpen(false);
    setSelectedText("");
  }, []);

  return { isOpen, selectedText, position, openNaturalEdit, closeNaturalEdit };
}

/**
 * Natural Edit 浮窗组件
 */
export function NaturalEditPanel({
  isOpen,
  selectedText,
  position,
  onClose,
  onApply,
}: {
  isOpen: boolean;
  selectedText: string;
  position: { x: number; y: number };
  onClose: () => void;
  onApply: (instruction: string) => void;
}) {
  const [instruction, setInstruction] = useState("");

  if (!isOpen) return null;

  return (
    <div
      className="fixed z-50 bg-white dark:bg-gray-800 border dark:border-gray-700 rounded-lg shadow-xl p-3 w-96"
      style={{ left: Math.min(position.x, window.innerWidth - 400), top: position.y }}
    >
      <div className="text-xs text-gray-500 dark:text-gray-400 mb-2">
        Selected: {selectedText.length} chars
      </div>
      <textarea
        className="w-full p-2 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-200"
        rows={3}
        placeholder="Describe the change you want..."
        value={instruction}
        onChange={(e) => setInstruction(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter" && !e.shiftKey) {
            e.preventDefault();
            onApply(instruction);
          }
          if (e.key === "Escape") {
            onClose();
          }
        }}
        autoFocus
      />
      <div className="flex items-center justify-between mt-2">
        <span className="text-xs text-gray-400">Enter to apply · Esc to cancel</span>
        <div className="flex gap-2">
          <button
            onClick={onClose}
            className="px-2 py-1 text-xs border rounded dark:border-gray-600 hover:bg-gray-100 dark:hover:bg-gray-700"
          >
            Cancel
          </button>
          <button
            onClick={() => onApply(instruction)}
            className="px-2 py-1 text-xs bg-blue-600 text-white rounded hover:bg-blue-700"
          >
            Apply
          </button>
        </div>
      </div>
    </div>
  );
}
