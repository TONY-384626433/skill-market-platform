"""SkillHub 沙箱行为审计钩子 (Python)

通过 PYTHONPATH 注入到被测技能进程, 用 sys.addaudithook 记录敏感行为:
  · 网络: socket.connect / getaddrinfo / gethostbyname / bind
  · 执行: subprocess.Popen / os.system / os.exec / os.spawn
  · 写入: open(写模式) / os.remove|unlink|rmdir
  · 读取: 仅记录疑似凭据/密钥类路径 (避免刷屏)

日志写入 $SANDBOX_TRACE (JSONL), 由沙箱服务解析成动态验证结论。
本钩子只做「观测」, 不阻断任何行为 —— 阻断由容器本身的隔离(无网络/只读根/非root)完成。
"""

import json
import os
import sys

_TRACE = os.environ.get("SANDBOX_TRACE")
_MAX_LINES = 4000

_SENSITIVE_HINTS = (
    ".ssh/id_", ".ssh\\id_", ".aws/credentials", ".netrc", "shadow", ".kube/config",
    "keychain", "cookies.sqlite", "wallet.dat", ".git-credentials", "id_rsa", "id_ed25519",
)

if _TRACE:
    try:
        _fh = open(_TRACE, "a", buffering=1, encoding="utf8")
    except OSError:
        _fh = None

    _state = {"count": 0}

    def _emit(event, target="", detail=""):
        if _fh is None or _state["count"] >= _MAX_LINES:
            return
        _state["count"] += 1
        try:
            _fh.write(json.dumps({
                "event": event,
                "target": str(target)[:200],
                "detail": str(detail)[:200],
                "pid": os.getpid(),
            }, ensure_ascii=False) + "\n")
        except Exception:
            pass

    def _is_sensitive(value):
        text = str(value).lower()
        for hint in _SENSITIVE_HINTS:
            if hint in text:
                return True
        return False

    def _hook(event, args):
        try:
            if event in ("socket.connect", "socket.getaddrinfo", "socket.gethostbyname", "socket.sendto"):
                _emit(event, args[0] if args else "")
            elif event == "socket.bind":
                _emit("socket.listen", args[0] if args else "", "bind")
            elif event == "subprocess.Popen":
                _emit("subprocess.Popen", (args[0] if args else ""), (args[1] if len(args) > 1 else ""))
            elif event in ("os.system", "os.exec", "os.spawn", "os.posix_spawn", "os.startfile"):
                _emit(event, args[0] if args else "")
            elif event == "open":
                path = args[0] if args else ""
                mode = args[1] if len(args) > 1 else "r"
                mode = mode if isinstance(mode, str) else "r"
                if any(flag in mode for flag in ("w", "a", "x", "+")):
                    _emit("open.write", path, mode)
                elif _is_sensitive(path):
                    _emit("open.read", path, mode)
            elif event in ("os.remove", "os.unlink", "os.rmdir"):
                _emit(event, args[0] if args else "")
            elif event == "shutil.rmtree":
                _emit("shutil.rmtree", args[0] if args else "")
        except Exception:
            pass

    try:
        sys.addaudithook(_hook)
    except Exception:
        pass
