#!/usr/bin/env python3
"""
Chiron 涓€閿繍琛岃剼鏈?鏀寔 Linux / Windows / macOS

鐢ㄦ硶:
    python run.py start          # 鍚姩鎵€鏈夋湇鍔?    python run.py start --bg     # 鍚庡彴鍚姩鎵€鏈夋湇鍔?    python run.py stop           # 鍋滄鎵€鏈夋湇鍔?    python run.py restart        # 閲嶅惎鎵€鏈夋湇鍔?    python run.py status         # 鏌ョ湅鏈嶅姟鐘舵€?    python run.py logs           # 鏌ョ湅鏃ュ織
    python run.py build          # 缂栬瘧 Go 鏈嶅姟
    python run.py setup          # 棣栨瀹夎渚濊禆
"""

import os
import sys
import json
import time
import signal
import shutil
import platform
import argparse
import subprocess
import socket
from pathlib import Path
from typing import Optional

# 鈹€鈹€ 閰嶇疆 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

BASE_DIR = Path(__file__).parent.resolve()
PID_DIR = BASE_DIR / ".pids"
LOG_DIR = BASE_DIR / "logs"
WORKSPACE_DIR = BASE_DIR / "workspace"

SERVICES = {
    "gateway": {
        "name": "Go Gateway",
        "port": 8080,
        "cmd": [str(BASE_DIR / ("Chiron.exe" if platform.system() == "Windows" else "Chiron"))],
        "env": {
            "PORT": "8080",
            "STORAGE_BACKEND": "local",
            "STORAGE_ROOT": str(WORKSPACE_DIR),
        },
    },
    "python-engine": {
        "name": "Python AI 寮曟搸",
        "port": 8000,
        "cmd": [sys.executable, "-m", "app.main"],
        "cwd": str(BASE_DIR / "python-engine"),
        "env": {
            "HTTP_PORT": "8000",
        },
    },
}

DEFAULT_ENV = {
    "LOG_LEVEL": "info",
    # 鐢熶骇閮ㄧ讲蹇呴』閫氳繃 .env 鏄惧紡璁剧疆 JWT_SECRET锛堚墺32 瀛楃锛岄殢鏈虹敓鎴愶級
    # 鏈厤缃椂鏈嶅姟灏嗘嫆缁濆惎鍔紙Go 缃戝叧 config.go 浼氭娴嬪苟 fail-fast锛?    "JWT_SECRET": "",  # 蹇呴』閫氳繃 .env 璁剧疆锛岀┖鍊兼椂 Go 缃戝叧浼氭嫆缁濆惎鍔?(fail-fast)
    "POSTGRES_DSN": "postgres://Chiron:Chiron@localhost:5432/Chiron?sslmode=disable",
    "REDIS_ADDR": "localhost:6379",
    "PYTHON_ENGINE_ADDRESS": "localhost:8000",
    # 鎻掍欢 per-user 閰嶇疆鐩綍锛氫笌 Go 缃戝叧 PLUGIN_DATA_DIR 鎸囧悜鍚屼竴浣嶇疆
    "PLUGIN_DATA_DIR": str(BASE_DIR / "data" / "plugins"),
    # 鍐呴儴绔偣鍏变韩瀵嗛挜锛圙o 缃戝叧 LLM_GATEWAY_KEY锛屾彃浠?reload 绛夋牎楠岋級
    "LLM_GATEWAY_KEY": "",
}


def load_env_file():
    """浠?.env 鏂囦欢鍔犺浇鐜鍙橀噺锛岃鐩?DEFAULT_ENV 涓殑榛樿鍊?""
    env_file = BASE_DIR / ".env"
    if not env_file.exists():
        return
    for line in env_file.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        if "=" in line:
            key, _, value = line.partition("=")
            key = key.strip()
            value = value.strip()
            # 鍙鐩?DEFAULT_ENV 涓凡鏈夌殑閿紝鎴栨坊鍔犳柊鐨勭幆澧冨彉閲?            if key:
                DEFAULT_ENV[key] = value


# 鍚姩鏃跺姞杞?.env 鏂囦欢
load_env_file()


# 鈹€鈹€ 宸ュ叿鍑芥暟 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

def is_windows() -> bool:
    return platform.system() == "Windows"


def is_port_open(port: int, host: str = "localhost", timeout: float = 1.0) -> bool:
    """妫€鏌ョ鍙ｆ槸鍚﹀湪鐩戝惉"""
    try:
        with socket.create_connection((host, port), timeout=timeout):
            return True
    except (ConnectionRefusedError, TimeoutError, OSError):
        return False


def gateway_needs_rebuild(exe_path: Path) -> bool:
    """Go 缃戝叧婧愮爜锛坈md/Chiron + internal锛夋槸鍚︽瘮缂栬瘧浜х墿鏂般€?
    run.py start 鍙湪 exe 缂哄け鏃惰嚜鍔ㄦ瀯寤猴紱鏀瑰姩 Go 婧愮爜鍚庤嫢娌跨敤鏃?exe锛?    浼氬惎鍔ㄨ繃鏈熺増鏈紙渚嬪鏁版嵁搴撻檷绾?瀹夎妯″紡淇鏈敓鏁堬級銆傝繖閲屽姣?mtime锛?    浠讳竴 .go 鏂囦欢姣?exe 鏂板垯杩斿洖 True锛堟彁绀洪噸寤猴級銆?    """
    if not exe_path.exists():
        return True
    exe_mtime = exe_path.stat().st_mtime
    for root in (BASE_DIR / "cmd" / "Chiron", BASE_DIR / "internal"):
        if not root.exists():
            continue
        try:
            for p in root.rglob("*.go"):
                if p.stat().st_mtime > exe_mtime:
                    return True
        except OSError:
            continue
    return False


def get_pid_file(service: str) -> Path:
    return PID_DIR / f"{service}.pid"


def read_pid(service: str) -> Optional[int]:
    pf = get_pid_file(service)
    if pf.exists():
        try:
            return int(pf.read_text().strip())
        except ValueError:
            return None
    return None


def write_pid(service: str, pid: int):
    PID_DIR.mkdir(parents=True, exist_ok=True)
    get_pid_file(service).write_text(str(pid))


def remove_pid(service: str):
    pf = get_pid_file(service)
    if pf.exists():
        pf.unlink()


def is_process_running(pid: int) -> bool:
    """妫€鏌ヨ繘绋嬫槸鍚﹀湪杩愯"""
    try:
        if is_windows():
            result = subprocess.run(
                ["tasklist", "/FI", f"PID eq {pid}"],
                capture_output=True, text=True
            )
            return str(pid) in result.stdout
        else:
            os.kill(pid, 0)
            return True
    except (ProcessLookupError, PermissionError, OSError):
        return False


def kill_process(pid: int):
    """缁堟杩涚▼"""
    try:
        if is_windows():
            subprocess.run(["taskkill", "/F", "/PID", str(pid)],
                          capture_output=True, timeout=5)
        else:
            os.kill(pid, signal.SIGTERM)
            time.sleep(1)
            if is_process_running(pid):
                os.kill(pid, signal.SIGKILL)
    except (ProcessLookupError, PermissionError, OSError, subprocess.TimeoutExpired):
        pass


def get_log_file(service: str, stream: str = "stdout") -> Path:
    return LOG_DIR / f"{service}.{stream}.log"


def color(text: str, code: str) -> str:
    """ANSI 棰滆壊杈撳嚭"""
    if is_windows():
        # Windows 10+ 鏀寔 ANSI
        try:
            import ctypes
            kernel32 = ctypes.windll.kernel32
            kernel32.SetConsoleMode(kernel32.GetStdHandle(-11), 7)
        except Exception:
            return text
    return f"\033[{code}m{text}\033[0m"


def green(text: str) -> str:
    return color(text, "32")


def red(text: str) -> str:
    return color(text, "31")


def yellow(text: str) -> str:
    return color(text, "33")


def blue(text: str) -> str:
    return color(text, "34")


def bold(text: str) -> str:
    return color(text, "1")


def gray(text: str) -> str:
    return color(text, "90")


# 鈹€鈹€ 鏈嶅姟绠＄悊 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

class ServiceManager:
    def __init__(self):
        PID_DIR.mkdir(parents=True, exist_ok=True)
        LOG_DIR.mkdir(parents=True, exist_ok=True)
        WORKSPACE_DIR.mkdir(parents=True, exist_ok=True)
        (WORKSPACE_DIR / "skills").mkdir(exist_ok=True)

    def _build_env(self, service_key: str) -> dict:
        """鏋勫缓鏈嶅姟鐜鍙橀噺"""
        env = os.environ.copy()
        # 鍙敞鍏ラ潪绌洪粯璁ゅ€硷細绌哄瓧绗︿覆娉ㄥ叆浼氳鐩?.env / 绯荤粺鐜涓凡鐢熸晥鐨勯厤缃?        # 锛堜緥濡?DEFAULT_ENV 鐨?JWT_SECRET="" 浼氳鐩?.env 鐨?JWT_SECRET锛?        # 瀵艰嚧 python-engine 鐨?pydantic 鏍￠獙鎷掔粷鍚姩锛?        env.update({k: v for k, v in DEFAULT_ENV.items() if v})
        if service_key in SERVICES:
            env.update(SERVICES[service_key].get("env", {}))
        return env

    def build(self):
        """缂栬瘧 Go 鏈嶅姟"""
        print(bold("缂栬瘧 Go Gateway..."))

        try:
            result = subprocess.run(
                ["go", "build", "-o",
                 "Chiron.exe" if is_windows() else "Chiron",
                 "./cmd/Chiron/"],
                cwd=str(BASE_DIR),
                capture_output=True, text=True
            )
        except FileNotFoundError:
            print(red("鏈壘鍒?go 鍛戒护锛岃鍏堝畨瑁?Go (https://go.dev/dl/)"))
            return False

        if result.returncode != 0:
            print(red(f"缂栬瘧澶辫触:\n{result.stderr}"))
            return False

        print(green("缂栬瘧鎴愬姛"))
        return True

    def _find_service_pid(self, key: str, port: int) -> Optional[int]:
        """鎵惧埌鐩戝惉鎸囧畾绔彛鐨勮繘绋?PID"""
        try:
            if is_windows():
                result = subprocess.run(
                    ["netstat", "-ano"],
                    capture_output=True, text=True
                )
                for line in result.stdout.splitlines():
                    if f":{port}" in line and "LISTENING" in line:
                        parts = line.split()
                        if parts:
                            try:
                                return int(parts[-1])
                            except ValueError:
                                pass
            else:
                result = subprocess.run(
                    ["lsof", "-i", f":{port}", "-t"],
                    capture_output=True, text=True
                )
                if result.stdout.strip():
                    return int(result.stdout.strip().split()[0])
        except Exception:
            pass
        return None

    def setup(self):
        """棣栨瀹夎渚濊禆"""
        print(bold("瀹夎渚濊禆..."))

        # 妫€鏌?Go
        try:
            result = subprocess.run(["go", "version"], capture_output=True, text=True)
            print(f"  Go: {result.stdout.strip()}")
        except FileNotFoundError:
            print(red("  Go 鏈畨瑁咃紝璇峰厛瀹夎 Go"))
            return False

        # 妫€鏌?Python
        print(f"  Python: {platform.python_version()}")

        # 缂栬瘧 Go
        if not self.build():
            return False


        # 妫€鏌?Python 渚濊禆
        print("  妫€鏌?Python 渚濊禆...")
        result = subprocess.run(
            [sys.executable, "-c", "import uvicorn; print('OK')"],
            capture_output=True, text=True
        )
        if "OK" not in result.stdout:
            print(yellow("  瀹夎 Python 渚濊禆..."))
            req_file = BASE_DIR / "python-engine" / "requirements.txt"
            if req_file.exists():
                subprocess.run(
                    [sys.executable, "-m", "pip", "install", "-r", str(req_file), "-q"],
                    capture_output=True
                )

        print(green("渚濊禆瀹夎瀹屾垚"))
        return True

    def _start_service(self, key: str, background: bool = True):
        """鍚姩鍗曚釜鏈嶅姟"""
        svc = SERVICES[key]
        name = svc["name"]
        port = svc["port"]
        cmd = svc["cmd"]
        cwd = svc.get("cwd", str(BASE_DIR))
        env = self._build_env(key)

        # 妫€鏌ユ槸鍚﹀凡鍦ㄨ繍琛?        pid = read_pid(key)
        if pid and is_process_running(pid):
            print(f"  {name}: {yellow('宸插湪杩愯')} (PID {pid})")
            return True

        # 妫€鏌ョ鍙ｅ崰鐢?        if is_port_open(port):
            print(f"  {name}: {yellow(f'绔彛 {port} 宸茶鍗犵敤')}")
            return False

        if not background:
            # 鍓嶅彴妯″紡
            print(f"  {name}: {blue('鍚姩涓?..')} (鍓嶅彴妯″紡, Ctrl+C 鍋滄)")
            try:
                proc = subprocess.Popen(cmd, cwd=cwd, env=env)
                write_pid(key, proc.pid)
                proc.wait()
            except KeyboardInterrupt:
                print(f"\n  {name}: 鍋滄涓?..")
            finally:
                remove_pid(key)
            return True

        # 鍚庡彴妯″紡
        stdout_log = get_log_file(key, "stdout")
        stderr_log = get_log_file(key, "stderr")

        if is_windows():
            # Windows: 鐩存帴浣跨敤 Popen 鍚庡彴鍚姩锛岄噸瀹氬悜杈撳嚭鍒版棩蹇楁枃浠?            out = open(stdout_log, "w")
            err = open(stderr_log, "w")
            proc = subprocess.Popen(
                cmd, cwd=cwd, env=env,
                stdout=out, stderr=err,
                creationflags=subprocess.DETACHED_PROCESS | subprocess.CREATE_NO_WINDOW,
                close_fds=True
            )
        else:
            # Linux/macOS: 浣跨敤 nohup
            out = open(stdout_log, "w")
            err = open(stderr_log, "w")
            proc = subprocess.Popen(
                cmd, cwd=cwd, env=env,
                stdout=out, stderr=err,
                start_new_session=True
            )

        write_pid(key, proc.pid)

        # 绛夊緟绔彛灏辩华锛涘瓙杩涚▼宸查€€鍑猴紙濡備緷璧栫己澶?閰嶇疆閿欒瀵艰嚧宕╂簝锛夋椂绔嬪嵆鎶ラ敊锛岄伩鍏嶇┖绛?        for _ in range(20):
            if is_port_open(port):
                # 鎵惧埌瀹為檯鐨勬湇鍔¤繘绋?PID
                actual_pid = self._find_service_pid(key, port)
                if actual_pid:
                    write_pid(key, actual_pid)
                print(f"  {name}: {green('宸插惎鍔?)} (PID {actual_pid or proc.pid}, 绔彛 {port})")
                return True
            if proc.poll() is not None:
                print(f"  {name}: {red('鍚姩澶辫触锛堣繘绋嬪凡閫€鍑猴級')} (閫€鍑虹爜 {proc.returncode})")
                remove_pid(key)
                return False
            time.sleep(1)

        print(f"  {name}: {yellow('宸插惎鍔ㄤ絾绔彛鏈氨缁?)} (PID {proc.pid})")
        return True

    def stop_service(self, key: str):
        """鍋滄鍗曚釜鏈嶅姟"""
        svc = SERVICES[key]
        name = svc["name"]

        pid = read_pid(key)
        if not pid:
            print(f"  {name}: {yellow('鏈繍琛?)}")
            return

        if not is_process_running(pid):
            print(f"  {name}: {yellow('宸插仠姝?)}")
            remove_pid(key)
            return

        print(f"  {name}: 鍋滄涓?(PID {pid})...")
        kill_process(pid)
        time.sleep(1)

        if is_process_running(pid):
            print(f"  {name}: {red('鍋滄澶辫触')}")
        else:
            print(f"  {name}: {green('宸插仠姝?)}")
        remove_pid(key)

    def start(self, background: bool = True, services: list = None):
        """鍚姩鏈嶅姟"""
        print(bold("\n鈺愨晲鈺?Chiron 鏈嶅姟鍚姩 鈺愨晲鈺怽n"))

        targets = services or list(SERVICES.keys())

        # gateway 渚濊禆缂栬瘧浜х墿 Chiron.exe锛涚己澶辨垨婧愮爜姣斾骇鐗╂柊鏃惰嚜鍔ㄦ瀯寤?        # 锛圧EADME: start 鑷姩鏋勫缓 Go锛涢伩鍏嶅惎鍔ㄨ繃鏈熺増鏈鑷翠慨澶嶆湭鐢熸晥锛?        if "gateway" in targets:
            exe_path = Path(SERVICES["gateway"]["cmd"][0])
            if gateway_needs_rebuild(exe_path):
                print(yellow("Go Gateway 闇€瑕佺紪璇戯紙浜х墿缂哄け鎴栨簮鐮佸凡鏇存柊锛?.."))
                if not self.build():
                    return False

        results = {}
        for key in targets:
            if key in SERVICES:
                results[key] = self._start_service(key, background)

        print()

        if background:
            self._print_summary()

        return all(results.values())

    def stop(self, services: list = None):
        """鍋滄鏈嶅姟"""
        print(bold("\n鈺愨晲鈺?Chiron 鏈嶅姟鍋滄 鈺愨晲鈺怽n"))

        targets = services or list(SERVICES.keys())
        for key in targets:
            if key in SERVICES:
                self.stop_service(key)
        print()

    def restart(self, services: list = None):
        """閲嶅惎鏈嶅姟"""
        self.stop(services)
        time.sleep(1)
        self.start(services=services)

    def status(self):
        """鏌ョ湅鏈嶅姟鐘舵€?""
        print(bold("\n鈺愨晲鈺?Chiron 鏈嶅姟鐘舵€?鈺愨晲鈺怽n"))
        print(f"{'鏈嶅姟':<20} {'鐘舵€?:<12} {'PID':<10} {'绔彛':<8} {'绔彛鐘舵€?}")
        print("鈹€" * 70)

        for key, svc in SERVICES.items():
            name = svc["name"]
            port = svc["port"]
            pid = read_pid(key)

            if pid and is_process_running(pid):
                port_ok = is_port_open(port)
                status_text = green("杩愯涓?)
                pid_text = str(pid)
                port_text = green("鐩戝惉涓?) if port_ok else yellow("鏈氨缁?)
            else:
                status_text = red("宸插仠姝?)
                pid_text = "-"
                port_text = gray("鏈洃鍚?) if not is_port_open(port) else yellow("琚崰鐢?)

            print(f"{name:<20} {status_text:<12} {pid_text:<10} {port:<8} {port_text}")

        print()

    def logs(self, service: str = None, follow: bool = False, tail: int = 50):
        """鏌ョ湅鏃ュ織"""
        if service:
            self._show_log(service, follow, tail)
        else:
            for key in SERVICES:
                self._show_log(key, follow=False, tail=10)

    def _show_log(self, key: str, follow: bool = False, tail: int = 50):
        """鏄剧ず鍗曚釜鏈嶅姟鐨勬棩蹇?""
        svc = SERVICES[key]
        name = svc["name"]

        print(bold(f"\n鈺愨晲鈺?{name} 鏃ュ織 鈺愨晲鈺怽n"))

        log_file = get_log_file(key, "stdout")
        if not log_file.exists():
            print(yellow("  鏆傛棤鏃ュ織"))
            return

        if follow:
            # 瀹炴椂璺熻釜
            print(f"  璺熻釜 {log_file} (Ctrl+C 閫€鍑?")
            try:
                proc = subprocess.Popen(
                    ["tail", "-f", str(log_file)] if not is_windows() else
                    ["powershell", "-Command", f"Get-Content '{log_file}' -Wait"],
                    stdout=subprocess.PIPE, text=True
                )
                for line in proc.stdout:
                    print(line, end="")
            except KeyboardInterrupt:
                proc.terminate()
        else:
            # 鏄剧ず鏈€鍚?N 琛?            lines = log_file.read_text(encoding="utf-8", errors="replace").splitlines()
            for line in lines[-tail:]:
                print(f"  {line}")

    def _print_summary(self):
        """鎵撳嵃鍚姩鎽樿"""
        print(bold("\n鈺愨晲鈺?鏈嶅姟璁块棶鍦板潃 鈺愨晲鈺怽n"))
        print(f"  Gateway:      http://localhost:{SERVICES['gateway']['port']}")
        print(f"  HTTP 寮曟搸:    http://localhost:8000")
        print()
        print(f"  鏃ュ織鐩綍:     {LOG_DIR}")
        print(f"  PID 鐩綍:     {PID_DIR}")
        print()
        print(f"  鍋滄鏈嶅姟:     python run.py stop")
        print(f"  鏌ョ湅鐘舵€?     python run.py status")
        print(f"  鏌ョ湅鏃ュ織:     python run.py logs")
        print()


# 鈹€鈹€ CLI 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

def main():
    parser = argparse.ArgumentParser(
        description="Chiron 涓€閿繍琛岃剼鏈?,
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
绀轰緥:
  python run.py setup           棣栨瀹夎渚濊禆
  python run.py build           缂栬瘧 Go 鏈嶅姟
  python run.py start           鍚庡彴鍚姩鎵€鏈夋湇鍔?  python run.py start --fg      鍓嶅彴鍚姩锛堣皟璇曠敤锛?  python run.py stop            鍋滄鎵€鏈夋湇鍔?  python run.py restart         閲嶅惎鎵€鏈夋湇鍔?  python run.py status          鏌ョ湅鏈嶅姟鐘舵€?  python run.py logs            鏌ョ湅鎵€鏈夋棩蹇?  python run.py logs gateway    鏌ョ湅 Gateway 鏃ュ織
        """
    )

    parser.add_argument("command", choices=[
        "setup", "build", "start", "stop", "restart", "status", "logs"
    ], help="鍛戒护")

    parser.add_argument("service", nargs="?", help="鎸囧畾鏈嶅姟 (gateway/python-engine)")
    parser.add_argument("--fg", action="store_true", help="鍓嶅彴妯″紡杩愯")
    parser.add_argument("--tail", type=int, default=50, help="鏃ュ織琛屾暟")

    args = parser.parse_args()

    mgr = ServiceManager()

    services = [args.service] if args.service else None

    if args.command == "setup":
        success = mgr.setup()
        sys.exit(0 if success else 1)

    elif args.command == "build":
        success = mgr.build()
        sys.exit(0 if success else 1)

    elif args.command == "start":
        success = mgr.start(background=not args.fg, services=services)
        sys.exit(0 if success else 1)

    elif args.command == "stop":
        mgr.stop(services)

    elif args.command == "restart":
        mgr.restart(services)

    elif args.command == "status":
        mgr.status()

    elif args.command == "logs":
        mgr.logs(service=args.service, tail=args.tail)


if __name__ == "__main__":
    main()

