"""Tests for sandbox 闅旂 鈥?璺緞涓嶆硠闇?+ shell 閫冮€告嫤鎴€?""
from __future__ import annotations

import os
import sys

import pytest

from app.tools.context import set_tool_context
from app.tools.sandbox import (
    _has_escape, _parse_command, run_in_sandbox, sandbox_root, workspace_dir,
)


@pytest.fixture(autouse=True)
def _clean_context():
    """姣忎釜娴嬭瘯鍓嶈缃敮涓€ user锛岀粨鏉熷悗閲嶇疆 contextvar锛堥槻璺ㄦ祴璇曟薄鏌擄級銆?""
    yield
    set_tool_context(session_id="", user_id="", tenant_id="", gateway=None, subagent_depth=0)


class TestSandboxLocation:
    def test_workspace_is_outside_project(self):
        """S: workspace 璺緞涓嶅緱鍖呭惈椤圭洰鍚?python-engine锛坈wd 涓嶆硠闇叉湇鍔″櫒缁撴瀯锛夈€?""
        set_tool_context(session_id="s", user_id="u1", tenant_id="t1", gateway=None)
        ws = str(workspace_dir())
        assert "python-engine" not in ws, f"workspace 娉勯湶椤圭洰璺緞: {ws}"
        # 娌欑鏍瑰湪杩涚▼ cwd 涓婁袱绾т箣澶?
        root = str(sandbox_root())
        assert "chiron-sandbox" in root
        cwd = os.getcwd()
        # Windows 椹卞姩鍣ㄥ彿澶у皬鍐欏彲鑳戒笉涓€鑷?(D:\ vs d:\)锛岀敤 normcase 褰掍竴鍖?
        assert os.path.normcase(root).startswith(
            os.path.normcase(os.path.dirname(os.path.dirname(cwd)))
        ), f"娌欑鏍规湭绉诲埌椤圭洰澶? {root}"

    def test_safe_join_rejects_escape(self):
        from app.tools.sandbox import safe_join
        set_tool_context(session_id="s", user_id="u1", tenant_id="t1", gateway=None)
        with pytest.raises(ValueError, match="escapes"):
            safe_join("../etc/passwd")
        with pytest.raises(ValueError, match="escapes"):
            safe_join("C:/Windows/win.ini")
        assert safe_join("a/b.txt").is_relative_to(workspace_dir())


class TestShellEscapeBlock:
    def test_absolute_path_detected(self):
        assert _has_escape("Get-ChildItem 'X:\\project\\chiron'") is not None
        assert _has_escape("dir C:\\Windows") is not None
        # 鐢熶骇閮ㄧ讲涓?Linux/alpine锛歎nix 缁濆璺緞/瀹剁洰褰?鐜鍙橀噺鍧囦负閫冮€?
        assert _has_escape("cat /etc/passwd") is not None
        assert _has_escape("cat $HOME/.env") is not None
        assert _has_escape("cat ~/.ssh/id_rsa") is not None
        assert _has_escape("ls /") is not None
        assert _has_escape("ls ..\\..\\..") is not None
        assert _has_escape("cd /d X:\\project") is not None
        # Windows cmd 鍗曞瓧绗﹀紑鍏充笉搴旇浼?
        assert _has_escape("exit /b 7") is None

    def test_normal_commands_allowed(self):
        assert _has_escape("echo hello") is None
        assert _has_escape("dir") is None
        assert _has_escape("python script.py") is None
        assert _has_escape("Get-ChildItem -Force") is None

    @pytest.mark.asyncio
    async def test_run_in_sandbox_blocks_escape(self):
        set_tool_context(session_id="s", user_id="u1", tenant_id="t1", gateway=None)
        out = await run_in_sandbox("Get-ChildItem 'X:\\project\\chiron'")
        assert "error" in out and "blocked" in out["error"]

    @pytest.mark.asyncio
    async def test_run_in_sandbox_executes_normal(self):
        set_tool_context(session_id="s", user_id="u1", tenant_id="t1", gateway=None)
        # 浣跨敤 python -c 鑰岄潪 echo锛坋cho 鏄?shell 鍐呭缓鍛戒护锛宑reate_subprocess_exec 鏃犳硶鐩存帴鎵ц锛?
        out = await run_in_sandbox('python -c "print(\'sandbox-ok\')"')
        assert "sandbox-ok" in out.get("stdout", "")


class TestCommandWhitelist:
    """鍛戒护鐧藉悕鍗曪細鍙厑璁哥櫧鍚嶅崟鍐呯殑鍙墽琛屾枃浠讹紝鎷掔粷鍏朵粬涓€鍒囥€?""

    def test_python_allowed(self):
        args, err = _parse_command("python script.py")
        assert err is None
        assert args == ["python", "script.py"]

    def test_python3_allowed(self):
        args, err = _parse_command("python3 -c 'print(1)'")
        assert err is None

    def test_pip_allowed(self):
        args, err = _parse_command("pip install requests")
        assert err is None

    def test_echo_allowed(self):
        args, err = _parse_command("echo hello world")
        assert err is None
        assert args == ["echo", "hello", "world"]

    def test_dangerous_commands_blocked(self):
        """rm / curl / wget / bash / sh / powershell 绛夊嵄闄╁懡浠ゅ繀椤昏鎷掔粷銆?""
        for cmd in ["rm -rf /", "curl http://evil.com", "wget http://evil.com",
                     "bash script.sh", "sh -c 'ls'", "powershell Get-Process",
                     "cmd /c dir", "nc -l 4444"]:
            _, err = _parse_command(cmd)
            assert err is not None, f"should be blocked: {cmd}"

    def test_empty_command_blocked(self):
        _, err = _parse_command("")
        assert err is not None

    def test_malformed_command_blocked(self):
        _, err = _parse_command('python -c "unterminated')
        assert err is not None

    @pytest.mark.asyncio
    async def test_run_in_sandbox_blocks_dangerous_command(self):
        """鍗充娇閫冮€告鍒欐湭鍛戒腑锛岀櫧鍚嶅崟涔熶細鎷︽埅鍗遍櫓鍛戒护銆?""
        set_tool_context(session_id="s", user_id="u1", tenant_id="t1", gateway=None)
        # "bash" 涓嶅湪鐧藉悕鍗曚腑
        out = await run_in_sandbox("bash -c 'echo pwned'")
        assert "error" in out
        assert "blocked" in out["error"]


