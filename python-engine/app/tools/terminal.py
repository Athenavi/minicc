"""persistent_shell 宸ュ叿 鈥?鎸佷箙缁堢浼氳瘽锛堝搴?deepseek-harness tool-bash-persistent锛?

姣忎釜 session 涓€涓暱椹?shell 杩涚▼锛歝wd銆佸鍑哄彉閲忋€佹縺娲荤幆澧冦€佸嚱鏁般€佸悗鍙颁换鍔?
璺ㄨ皟鐢ㄦ寔涔呫€?

瀹炵幇瑕佺偣锛氶潪浜や簰 shell 缁忕閬?stdout 鏄叏缂撳啿锛?KB锛宺eadline 鏀朵笉鍒板皬杈撳嚭锛?
deepseek 鐢?PTY 瑙ｅ喅锛夛紝鏁呭懡浠よ緭鍑洪噸瀹氬悜鍒颁复鏃舵枃浠躲€佺敤鍝ㄥ叺鏂囦欢鏍囪瀹屾垚锛?
瑙勯伩缂撳啿闂涓斾繚鎸?shell 鐘舵€佹寔涔呫€傝秴鏃?宕╂簝鏃跺叧闂?shell锛屼笅娆¤皟鐢ㄩ噸寤?
锛堜笌 deepseek 鐨?reset 璇箟涓€鑷达級銆?
"""
from __future__ import annotations

import asyncio
import logging
import os
import sys
import tempfile
import uuid
from pathlib import Path
from typing import Any, Optional

from app.tools.context import get_session_id
from app.tools.registry import registry
from app.tools.sandbox import sandboxed_env, workspace_dir

logger = logging.getLogger(__name__)

DEFAULT_TIMEOUT_SECONDS = 120


def _shell_command() -> str:
    if sys.platform == "win32":
        return os.environ.get("COMSPEC", "cmd.exe")
    return os.environ.get("SHELL", "/bin/bash")


class PersistentTerminal:
    """杩涚▼鍐呮寔涔?shell 绠＄悊鍣紝鎸?session key 闅旂銆?""

    def __init__(self) -> None:
        self._procs: dict[str, asyncio.subprocess.Process] = {}

    def _prune_procs(self) -> None:
        """鍥炴敹宸查€€鍑虹殑杩涚▼鏉＄洰锛岄槻姝?_procs 鏃犻檺绱Н锛圫 璧勬簮淇锛夈€?""
        dead = [k for k, p in self._procs.items() if p is not None and p.returncode is not None]
        for k in dead:
            self._procs.pop(k, None)

    async def _get_proc(self, key: str) -> asyncio.subprocess.Process:
        proc = self._procs.get(key)
        if proc is not None and proc.returncode is None:
            return proc
        # 鍥炴敹宸查€€鍑鸿繘绋嬶紝鍐嶅垱寤烘柊鐨勶紙閬垮厤绱Н瀹屾垚/宕╂簝鐨?shell锛?
        self._prune_procs()
        # S 淇锛歸s 鐢ㄥ眬閮ㄥ彉閲忥紝閬垮厤鍐欐垚瀹炰緥灞炴€у悗琚苟鍙戣姹傝鐩栧鑷磋法浼氳瘽涓?cwd/cd
        ws = str(workspace_dir())
        shell_exe = _shell_command()
        proc = await asyncio.create_subprocess_exec(
            shell_exe,
            stdin=asyncio.subprocess.PIPE,
            stdout=asyncio.subprocess.DEVNULL,
            stderr=asyncio.subprocess.DEVNULL,
            cwd=ws,
            # S 瀹夊叏淇锛氭竻鐞嗗涓?env(API key/JWT_SECRET/internal token 绛?锛?
            # 浠呬繚鐣?PATH/HOME 绛夊熀纭€鍙橀噺骞堕噸瀹氬悜鍒版矙绠憋紝闃叉妯″瀷 `env` 澶栧甫瀵嗛挜銆?
            env=sandboxed_env(),
        )
        # 棣栧懡浠?cd 鍒版矙绠?workspace锛屼繚璇佺姸鎬佹寔涔呭湪闅旂鐩綍鍐咃紙S 瀹夊叏淇锛?
        try:
            if sys.platform == 'win32':
                proc.stdin.write(f'cd /d \"{ws}\"\n'.encode('utf-8'))
            else:
                proc.stdin.write(f'cd \"{ws}\"\n'.encode('utf-8'))
            await proc.stdin.drain()
        except Exception:
            pass
        self._procs[key] = proc
        return proc

    async def execute(self, key: str, command: str, timeout: int = DEFAULT_TIMEOUT_SECONDS) -> dict[str, Any]:
        proc = await self._get_proc(key)

        # 閫冮€告嫤鎴細涓?shell_exec 鍚屼竴濂楄鍒欙紙缁濆璺緞/鐖剁洰褰?浜戝厓鏁版嵁锛夛紝
        # 鍚﹀垯鎸佷箙 shell 鍙洿鎺?cat /etc/passwd 绛夛紙S 瀹夊叏淇锛?
        from app.tools.sandbox import _has_escape
        esc = _has_escape(command)
        if esc:
            return {"output": f"[blocked: {esc}] command not allowed in sandbox", "exit_code": -1, "persistent": True}

        # 姣忓懡浠や竴涓复鏃跺伐浣滅洰褰曪細out=杈撳嚭 / code=閫€鍑虹爜 / done=瀹屾垚鍝ㄥ叺
        workdir = Path(tempfile.mkdtemp(prefix="chiron_sh_"))
        out_file = workdir / "out.txt"
        code_file = workdir / "code.txt"
        done_file = workdir / "done.txt"

        if sys.platform == "win32":
            lines = (
                f'{command} > "{out_file}" 2>&1',
                f'echo %errorlevel% > "{code_file}"',
                f'type nul > "{done_file}"',
            )
        else:
            lines = (
                f'{command} > "{out_file}" 2>&1',
                f'echo $? > "{code_file}"',
                f'touch "{done_file}"',
            )

        try:
            proc.stdin.write("\n".join(lines).encode("utf-8") + b"\n")
            await proc.stdin.drain()
        except (BrokenPipeError, ConnectionResetError, ValueError):
            await self._discard(key)
            return {"output": "[shell exited 鈥?reset]", "exit_code": -1, "persistent": False, "reset": True}

        # 杞瀹屾垚鍝ㄥ叺
        elapsed = 0.0
        while elapsed < timeout:
            if done_file.exists():
                break
            if proc.returncode is not None:
                await self._discard(key)
                return {"output": f"[shell exited: code {proc.returncode} 鈥?reset]", "exit_code": proc.returncode or 0, "persistent": False, "reset": True}
            await asyncio.sleep(0.05)
            elapsed += 0.05
        else:
            # 瓒呮椂锛氬叧闂笉纭畾鐨?shell锛屼笅娆￠噸寤?
            await self._discard(key)
            partial = out_file.read_text(encoding="utf-8", errors="replace") if out_file.exists() else ""
            return {
                "output": (partial + "\n" if partial else "") + f"[timed out after {timeout}s 鈥?shell reset]",
                "exit_code": -1,
                "persistent": False,
                "reset": True,
            }

        output = out_file.read_text(encoding="utf-8", errors="replace") if out_file.exists() else ""
        exit_code = 0
        try:
            exit_code = int(code_file.read_text(encoding="utf-8").strip() or 0)
        except (OSError, ValueError):
            pass
        return {"output": output, "exit_code": exit_code, "persistent": True}

    async def _discard(self, key: str) -> None:
        proc = self._procs.pop(key, None)
        if proc is not None and proc.returncode is None:
            try:
                proc.kill()
                await proc.wait()
            except Exception:  # noqa: BLE001
                pass

    async def close_all(self) -> None:
        for key in list(self._procs.keys()):
            await self._discard(key)


# 杩涚▼鍐呭崟渚?
_terminal = PersistentTerminal()


async def persistent_shell(command: str, timeout: int = DEFAULT_TIMEOUT_SECONDS) -> dict[str, Any]:
    """Run *command* in the session's persistent shell.

    cwd, exported variables, activated environments, functions and background
    jobs persist across calls in the same session. A timeout or shell exit
    resets the shell; the next call starts fresh (the result says so).
    """
    if not command.strip():
        return {"error": "command is required"}
    key = get_session_id() or "default"
    return await _terminal.execute(key, command, timeout=timeout)


registry.register(
    name="persistent_shell",
    description=(
        "Run a command in a persistent shell shared across calls in this session. "
        "State (cwd, exports, activated venv, functions, background jobs) persists "
        "between calls 鈥?use `cd` once and it stays. Prefer this over shell_exec "
        "for multi-step work. "
        "The command's stdout/stderr are captured and returned as `output`; "
        "do NOT use `>`/`2>` redirection to files inside the command (it is "
        "captured by the tool) 鈥?save files with the write_file tool instead. "
        "A timeout or shell crash resets the shell and the next call starts fresh."
    ),
    parameters={
        "type": "object",
        "properties": {
            "command": {"type": "string", "description": "Shell command to run"},
            "timeout": {"type": "integer", "default": DEFAULT_TIMEOUT_SECONDS, "description": "Seconds before the shell is reset"},
        },
        "required": ["command"],
    },
    handler=persistent_shell,
)

