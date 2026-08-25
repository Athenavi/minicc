"""sandbox 鈥?per-user 杩愯鐜闅旂锛圓 灞傦細浠ｇ爜绾?root/cwd 閿佸畾锛?

鎵€鏈?agent 鏂囦欢/鍛戒护宸ュ叿寮哄埗鍦?`SANDBOX_ROOT/{tenant}/{user}/workspace/` 鍐?
鎵ц锛屾ā鍨嬫棤娉曟帴瑙﹀涓绘枃浠剁郴缁燂紙瑙ｅ喅锛氭€濊€?杈撳嚭鏆撮湶鏈嶅姟鍣ㄧ湡瀹炶矾寰勶級銆?

- get_sandbox_dir() / workspace_dir()锛氱敱 contextvars锛坲ser/tenant锛夋帹瀵?
- safe_join()锛氱浉瀵硅矾寰?clamp 鍒?workspace锛屾嫆缁?../ 涓庣粷瀵硅矾寰勯€冮€?
"""
from __future__ import annotations

import asyncio
import logging
import os
import shlex
import sys
from pathlib import Path
from typing import Any

logger = logging.getLogger(__name__)


def sandbox_root() -> Path:
    """娌欑鏍癸細榛樿缃簬杩涚▼ cwd 涓婁袱绾э紙椤圭洰澶栵級锛岄伩鍏?workspace 璺緞
    娉勯湶鏈嶅姟鍣ㄩ」鐩粨鏋勶紙S 瀹夊叏淇锛歝wd 娉勯湶椤圭洰/python-engine 鍓嶇紑锛夈€?""
    env = os.environ.get("SANDBOX_ROOT")
    if env:
        return Path(env).resolve()
    # 鏃犺 cwd 鏄」鐩牴杩樻槸瀛愮洰褰曪紙python-engine锛夛紝涓婁袱绾ч兘鍒伴」鐩
    return Path(os.getcwd()).resolve().parent.parent / "chiron-sandbox"


def get_sandbox_dir() -> Path:
    """褰撳墠 user 鐨勬矙绠辨牴锛堢敱 context 鎺ㄥ锛岃嚜鍔ㄥ垱寤猴級銆?""
    from app.tools.context import get_user_id, get_tenant_id
    tenant = get_tenant_id() or "default"
    user = get_user_id() or "anonymous"
    d = sandbox_root() / tenant / user
    try:
        d.mkdir(parents=True, exist_ok=True)
    except OSError as e:
        logger.warning("sandbox dir create failed: %s", e)
    return d


def workspace_dir() -> Path:
    """agent 鏂囦欢鎿嶄綔鐨勫敮涓€鏍圭洰褰曘€?""
    d = get_sandbox_dir() / "workspace"
    try:
        d.mkdir(parents=True, exist_ok=True)
    except OSError as e:
        logger.warning("workspace dir create failed: %s", e)
    return d


def safe_join(rel: str) -> Path:
    """灏嗙浉瀵硅矾寰?clamp 鍒?workspace锛屾嫆缁濋€冮€革紙缁濆璺緞/../锛夈€?""
    base = workspace_dir().resolve()
    target = (base / rel).resolve()
    if target != base and not str(target).startswith(str(base) + os.sep):
        raise ValueError(f"path escapes sandbox: {rel}")
    return target


def sandboxed_env() -> dict[str, str]:
    """鎵ц鐜锛氭竻鐞嗘晱鎰?瀹夸富鍙橀噺锛屼粎淇濈暀鍩虹 PATH锛屽苟鎶婄敤鎴风洰褰曞彉閲?
    閲嶅畾鍚戝埌娌欑鍐咃紝闃叉 `$HOME`/`~` 娉勬紡瀹夸富璺緞锛圫 瀹夊叏淇锛夈€?""
    allow = {"PATH", "SYSTEMROOT", "WINDIR", "COMSPEC", "TEMP", "TMP", "HOME", "USERPROFILE"}
    env = {k: v for k, v in os.environ.items() if k in allow}
    env["HOME"] = str(workspace_dir())
    env["USERPROFILE"] = str(workspace_dir())
    env["TEMP"] = str(get_sandbox_dir())
    env["TMP"] = str(get_sandbox_dir())
    return env


# 琚姝㈢殑 shell 閫冮€告ā寮忥紙S 瀹夊叏淇锛歝wd 閿佸畾鏃犳硶闃绘鍛戒护鍐呯粷瀵硅矾寰勮闂涓伙級
# 娉ㄦ剰锛氱敓浜ч儴缃蹭负 Linux/alpine锛堣 Dockerfile锛夛紝蹇呴』瑕嗙洊 Unix 缁濆璺緞銆?
_ESCAPE_PATTERNS = [
    r"[A-Za-z]:[\\/]",        # Windows 鐩樼缁濆璺緞 (C:\ 鎴?C:/)
    r"\\\\",                  # UNC 璺緞
    r"(^|[^A-Za-z0-9_.])(\.\.)[\\/]",  # 鐖剁洰褰曡烦杞?..\ 鎴?../
    r"\bcd\b[^&|;]*[A-Za-z]:[\\/]",   # cd 鍒扮粷瀵硅矾寰?
    # Unix/Linux 閫冮€革細缁濆璺緞锛?xxx锛?+ 瀛楃璺緞娈垫垨鍗曠嫭 /锛夈€亊 瀹剁洰褰曘€?HOME 鍙橀噺銆?
    # 浠ョ┖鏍?寮曞彿/鍒嗗彿寮€澶寸晫瀹氾紝閬垮厤璇激鐩稿璺緞锛坅/b锛夈€乁RL锛坔ttp://锛?
    # 涓?Windows cmd 鍗曞瓧绗﹀紑鍏筹紙/b /d /s锛夈€?
    r"(?:^|[\s\"';])(?:/(?:[A-Za-z0-9_.-]{2,}|$)|~/?|\$\{?HOME\}?\b)",
    # 浜戝厓鏁版嵁 / 鍐呯綉鍦板潃锛坈url/wget 绛?SSRF 甯歌鐩爣锛?
    r"(?:^|[\s\"';])(?:curl|wget)\s+https?://(?:169\.254\.169\.254|127\.0\.0\.1|localhost)",
]

# 鍏佽鍦ㄦ矙绠卞唴鎵ц鐨勫懡浠ょ櫧鍚嶅崟锛堜粎鍏佽瀹夊叏鐨?Python/鏁版嵁鎿嶄綔鍛戒护锛?
_ALLOWED_EXECUTABLES: set[str] = {
    # Python 瑙ｉ噴鍣?
    "python", "python3",
    # 瀹夊叏鐨勫熀纭€鍛戒护
    "echo", "ls", "dir", "cat", "type", "head", "tail", "wc",
    "find", "grep", "sort", "uniq", "cut", "tr", "tee",
}


def _normalize_exe(name: str) -> str:
    """瑙勮寖鍖栧彲鎵ц鏂囦欢鍚嶏細鍙?basename锛屽幓骞冲彴鍚庣紑锛屽皬鍐欍€?""
    name = Path(name).name.lower()
    for suffix in (".exe", ".cmd", ".bat", ".com"):
        if name.endswith(suffix):
            name = name[: -len(suffix)]
            break
    return name


def _parse_command(command: str) -> tuple[list[str], str | None]:
    """灏嗗懡浠ゅ瓧绗︿覆瑙ｆ瀽涓?[executable, *args]锛屾牎楠岀櫧鍚嶅崟銆?

    杩斿洖 (args, error_reason)銆俥rror_reason 闈?None 琛ㄧず琚嫆缁濄€?
    """
    command = command.strip()
    if not command:
        return [], "empty command"

    try:
        parts = shlex.split(command)
    except ValueError as e:
        return [], f"invalid command syntax: {e}"

    if not parts:
        return [], "empty command"

    exe_name = _normalize_exe(parts[0])

    # 鐧藉悕鍗曟牎楠岋紙python/python3 宸插湪 _ALLOWED_EXECUTABLES 涓紝
    # sys.executable 瀹屾暣璺緞鐢?_normalize_exe 瑙勮寖鍖栧悗缁熶竴璧扮櫧鍚嶅崟鏍￠獙锛?
    if exe_name not in _ALLOWED_EXECUTABLES:
        return [], f"executable not allowed: {exe_name}"

    return parts, None


def _has_escape(command: str) -> str | None:
    """妫€娴嬪懡浠ゆ槸鍚﹀惈閫冮€告ā寮忥紝鍛戒腑杩斿洖鍘熷洜銆?""
    import re
    for pat in _ESCAPE_PATTERNS:
        m = re.search(pat, command)
        if m:
            return pat
    return None


async def run_in_sandbox(command: str, timeout: int = 120) -> dict[str, Any]:
    """鍦ㄦ矙绠?workspace 鍐呮墽琛屽懡浠わ紙direct exec锛屼笉缁忚繃 shell锛夛紝
    cwd 閿佸畾 + 鐜娓呯悊 + 閫冮€告嫤鎴?+ 鍛戒护鐧藉悕鍗曘€?

    浣跨敤 create_subprocess_exec 鏇夸唬 create_subprocess_shell锛?
    褰诲簳娑堥櫎 shell 鍏冨瓧绗︽敞鍏ワ紙绠￠亾/閲嶅畾鍚?鍙橀噺灞曞紑/閫氶厤绗︾瓑锛夈€?
    """
    import asyncio.subprocess

    # 绗竴灞傦細姝ｅ垯閫冮€告嫤鎴紙缁濆璺緞/鐖剁洰褰?SSRF锛?
    esc = _has_escape(command)
    if esc:
        return {
            "error": f"command blocked: absolute-path / parent-directory access is not allowed in sandbox",
            "reason": esc,
        }

    # 绗簩灞傦細瑙ｆ瀽鍛戒护 + 鐧藉悕鍗曟牎楠?
    args, reason = _parse_command(command)
    if reason:
        return {"error": f"command blocked: {reason}"}

    prog = args[0]
    rest = args[1:]

    # Windows: echo/dir/type 绛夋槸 cmd.exe 鍐呯疆鍛戒护锛屾棤鐙珛鍙墽琛屾枃浠讹紝
    # create_subprocess_exec 鐩存墽浼?WinError 2锛涚敤 cmd /c 鍖呰９鎵ц
    # 锛坅rgv 宸茬粡鐧藉悕鍗?+ 閫冮€告嫤鎴紝shlex 瑙ｆ瀽鏃?shell 鍏冨瓧绗︼紝涓嶆柊澧炴敞鍏ラ潰锛夈€?
    exe_name = _normalize_exe(prog)
    if sys.platform == "win32" and exe_name not in ("python", "python3"):
        prog, rest = os.environ.get("COMSPEC", "cmd.exe"), ["/d", "/s", "/c", *args]

    proc = await asyncio.create_subprocess_exec(
        prog, *rest,
        cwd=str(workspace_dir()),
        env=sandboxed_env(),
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
    )
    try:
        stdout, stderr = await asyncio.wait_for(proc.communicate(), timeout=timeout)
    except asyncio.TimeoutError:
        proc.kill()
        await proc.wait()
        return {"error": "timeout", "timeout": timeout}
    return {
        "exit_code": proc.returncode,
        "stdout": stdout.decode("utf-8", errors="replace"),
        "stderr": stderr.decode("utf-8", errors="replace"),
    }

