#!/usr/bin/env python3
"""SkillHub 沙箱网关 (TCP 代理)

为什么需要它:
  动态沙箱容器必须处于「无外网出口」的网络里 (network internal), 这样被测技能
  任何外联都不可达; 但 Docker Desktop 下 internal 网络不发布端口, 宿主无法直接访问。
  于是用这个网关容器做二层转发:
      宿主/localhost:8090  ->  skillhub-sandbox-gw  ->  (inner net) skillhub-sandbox:8090

安全说明:
  · 网关只运行本文件 (纯转发), 不执行任何被测代码;
  · 网关是可被宿主访问的一侧, 但沙箱无法借它出网 (被测程序只能连到网关的该端口,
    流量会被转发回沙箱自身, 不构成出口);
  · 生产环境建议把沙箱放到独立主机/独立 rootless 守护进程, 或使用集群 Job 隔离。
"""

import os
import socket
import threading

LISTEN_PORT = int(os.environ.get("PROXY_PORT", "8090"))
TARGET_HOST = os.environ.get("PROXY_TARGET_HOST", "sandbox")
TARGET_PORT = int(os.environ.get("PROXY_TARGET_PORT", "8090"))
IDLE_TIMEOUT = float(os.environ.get("PROXY_TIMEOUT", "180"))
MAX_CONN = int(os.environ.get("PROXY_MAX_CONN", "32"))

_sem = threading.BoundedSemaphore(MAX_CONN)


def pipe(src, dst):
    try:
        while True:
            chunk = src.recv(65536)
            if not chunk:
                break
            dst.sendall(chunk)
    except OSError:
        pass
    finally:
        for sock in (src, dst):
            try:
                sock.shutdown(socket.SHUT_RDWR)
            except OSError:
                pass


def handle(client):
    try:
        upstream = socket.create_connection((TARGET_HOST, TARGET_PORT), timeout=10)
    except OSError:
        try:
            client.sendall(b"HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n")
        except OSError:
            pass
        client.close()
        return
    client.settimeout(IDLE_TIMEOUT)
    upstream.settimeout(IDLE_TIMEOUT)
    t1 = threading.Thread(target=pipe, args=(client, upstream), daemon=True)
    t2 = threading.Thread(target=pipe, args=(upstream, client), daemon=True)
    t1.start()
    t2.start()
    t1.join()
    t2.join()
    for sock in (client, upstream):
        try:
            sock.close()
        except OSError:
            pass


def main():
    server = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    server.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    server.bind(("0.0.0.0", LISTEN_PORT))
    server.listen(64)
    print(f"[sandbox-gw] 监听 :{LISTEN_PORT} -> {TARGET_HOST}:{TARGET_PORT}", flush=True)
    while True:
        client, _ = server.accept()
        if not _sem.acquire(blocking=False):
            client.close()
            continue

        def run(conn=client):
            try:
                handle(conn)
            finally:
                _sem.release()

        threading.Thread(target=run, daemon=True).start()


if __name__ == "__main__":
    main()
