"""Python 本地工具注册表与核心工具的最小回归测试。"""
import asyncio
import pytest

from app.tools.registry import registry
import app.tools.core  # noqa: F401
import app.tools.memory  # noqa: F401
import app.tools.pm  # noqa: F401
import app.tools.skill  # noqa: F401
import app.tools.graph  # noqa: F401
import app.tools.agent  # noqa: F401
import app.tools.browser  # noqa: F401
import app.tools.media  # noqa: F401
import app.tools.edit_file  # noqa: F401
import app.tools.glob_tools  # noqa: F401
import app.tools.git_tools  # noqa: F401
import app.workflow  # noqa: F401


def test_registry_lists_core_tools():
    names = set(registry.list_names())
    for expected in [
        "read_file", "write_file", "shell_exec", "grep_files", "web_fetch",
        "search_files", "execute_python",
        "workflow_run", "workflow_status",
        "remember", "recall", "forget",
        "prd_generate", "tech_design", "task_decompose", "requirement_validate",
        "skill_list", "skill_install", "skill_generate", "skill_discover",
        "graph_create", "graph_run", "graph_templates", "workflow_run",
        "agent_dispatch", "agent_list", "code_agent", "agent_session_create", "agent_session_list",
        "browser_navigate", "browser_click", "browser_type", "browser_read", "browser_screenshot",
        "browser_scroll", "browser_get_state", "browser_tab_list", "browser_tab_create", "browser_tab_switch", "browser_tab_close",
        "media_create", "image_generate",
        "edit_file",
        "glob_files",
        "git_status", "git_diff", "git_log", "git_commit", "git_branch",
    ]:
        assert expected in names, f"{expected} not registered"


@pytest.mark.asyncio
async def test_read_file_missing_returns_error():
    result = await registry.execute("read_file", {"path": "no-such-file.txt"})
    assert "error" in result


@pytest.mark.asyncio
async def test_grep_files_returns_matches():
    result = await registry.execute("grep_files", {"query": "def ", "root": ".", "glob": "*.py", "max_results": 5})
    assert "count" in result
    assert result["count"] >= 0


@pytest.mark.asyncio
async def test_shell_exec_echo():
    result = await registry.execute("shell_exec", {"command": "echo ok"})
    assert result.get("exit_code") == 0
    assert "ok" in result.get("stdout", "")


@pytest.mark.asyncio
async def test_memory_remember_and_recall():
    await registry.execute("remember", {"key": "pytest:demo", "value": "works"})
    res = await registry.execute("recall", {"query": "pytest:demo"})
    assert "pytest:demo" in res.get("output", "")
    await registry.execute("forget", {"key": "pytest:demo"})


@pytest.mark.asyncio
async def test_pm_fallback_without_gateway():
    res = await registry.execute("prd_generate", {"description": "A simple todo app"})
    assert "output" in res
    assert "A simple todo app" in res["output"]


@pytest.mark.asyncio
async def test_skill_generate_without_install():
    res = await registry.execute("skill_generate", {"description": "summarize text"})
    assert res.get("name")
    assert res.get("type") == "prompt"


# ── edit_file tests ──────────────────────────────────────────────


@pytest.mark.asyncio
async def test_edit_file_exact_replacement(tmp_path):
    """Create a temp file, edit with old_string/new_string, verify content & diff."""
    f = tmp_path / "sample.txt"
    f.write_text("hello world\nfoo bar\nbaz\n", encoding="utf-8")

    result = await registry.execute("edit_file", {
        "path": "sample.txt",
        "old_string": "foo bar",
        "new_string": "foo BAR",
        "root": str(tmp_path),
    })

    assert result.get("success") is True, result
    # File content updated
    assert f.read_text(encoding="utf-8") == "hello world\nfoo BAR\nbaz\n"
    # Diff should mention both old and new
    diff = result["diff"]
    assert "-foo bar" in diff
    assert "+foo BAR" in diff


@pytest.mark.asyncio
async def test_edit_file_old_string_not_found(tmp_path):
    f = tmp_path / "x.txt"
    f.write_text("aaa\n", encoding="utf-8")
    result = await registry.execute("edit_file", {
        "path": "x.txt",
        "old_string": "zzz",
        "new_string": "yyy",
        "root": str(tmp_path),
    })
    assert "error" in result
    assert "not found" in result["error"]


@pytest.mark.asyncio
async def test_edit_file_non_unique_old_string(tmp_path):
    f = tmp_path / "dup.txt"
    f.write_text("abc\nabc\ndef\n", encoding="utf-8")
    result = await registry.execute("edit_file", {
        "path": "dup.txt",
        "old_string": "abc",
        "new_string": "XYZ",
        "root": str(tmp_path),
    })
    assert "error" in result
    assert "2 times" in result["error"]


@pytest.mark.asyncio
async def test_edit_file_line_range(tmp_path):
    f = tmp_path / "lines.txt"
    f.write_text("line1\nline2\nline3\nline4\n", encoding="utf-8")

    result = await registry.execute("edit_file", {
        "path": "lines.txt",
        "start_line": 2,
        "end_line": 3,
        "new_content": "replaced\n",
        "root": str(tmp_path),
    })

    assert result.get("success") is True, result
    assert f.read_text(encoding="utf-8") == "line1\nreplaced\nline4\n"
    diff = result["diff"]
    assert "-line2" in diff
    assert "+replaced" in diff


# ── glob_files tests ─────────────────────────────────────────────


@pytest.mark.asyncio
async def test_glob_finds_python_files(tmp_path):
    """Create temp dir with .py and .txt files, verify glob finds only .py."""
    (tmp_path / "hello.py").write_text("print('hi')\n", encoding="utf-8")
    (tmp_path / "world.py").write_text("x = 1\n", encoding="utf-8")
    (tmp_path / "notes.txt").write_text("not python\n", encoding="utf-8")
    sub = tmp_path / "sub"
    sub.mkdir()
    (sub / "deep.py").write_text("pass\n", encoding="utf-8")
    (sub / "data.csv").write_text("a,b\n", encoding="utf-8")

    result = await registry.execute("glob_files", {
        "pattern": "**/*.py",
        "root": str(tmp_path),
    })

    assert "files" in result, result
    paths = [f["path"] for f in result["files"]]
    assert len(paths) == 3, f"Expected 3 .py files, got {len(paths)}: {paths}"
    for p in paths:
        assert p.endswith(".py"), f"Non-py file in result: {p}"
    # txt and csv must NOT appear
    assert not any(p.endswith(".txt") for p in paths)
    assert not any(p.endswith(".csv") for p in paths)


# ── git tools tests ──────────────────────────────────────────────


@pytest.mark.asyncio
async def test_git_status_in_repo(tmp_path):
    """Create temp git repo, make changes, verify git_status returns changes."""
    import subprocess

    # Initialise a fresh repo
    subprocess.run(["git", "init"], cwd=str(tmp_path), check=True,
                   capture_output=True, timeout=10)
    subprocess.run(["git", "config", "user.email", "test@test.com"],
                   cwd=str(tmp_path), check=True, capture_output=True, timeout=10)
    subprocess.run(["git", "config", "user.name", "Test"],
                   cwd=str(tmp_path), check=True, capture_output=True, timeout=10)

    # Make an initial commit so the repo is clean
    (tmp_path / "README.md").write_text("init\n", encoding="utf-8")
    subprocess.run(["git", "add", "-A"], cwd=str(tmp_path), check=True,
                   capture_output=True, timeout=10)
    subprocess.run(["git", "commit", "-m", "init"], cwd=str(tmp_path),
                   check=True, capture_output=True, timeout=10)

    # Create new and modified files
    (tmp_path / "new_file.py").write_text("print('new')\n", encoding="utf-8")
    (tmp_path / "README.md").write_text("changed\n", encoding="utf-8")

    result = await registry.execute("git_status", {"root": str(tmp_path)})

    assert "files" in result, result
    assert result["count"] >= 2, f"Expected >=2 changed files, got {result['count']}"
    statuses = {f["path"]: f["status"] for f in result["files"]}
    assert "new_file.py" in statuses, f"new_file.py not in {statuses}"
    assert "README.md" in statuses, f"README.md not in {statuses}"
