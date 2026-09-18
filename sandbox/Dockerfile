# ============================================================
# SKILLHUB SANDBOX — 技能动态行为验证沙箱镜像
#
# 定位: 把待审技能放进「无网络 / 只读根文件系统 / 非 root / 丢弃全部
#       capabilities / 限制内存与进程数」的容器里真跑一次, 观测行为。
# 构建: docker build -t skillhub-sandbox ./sandbox
# 说明: 仅 COPY 代码, 不联网装包 (基础镜像复用本地 python:3.12-slim)
# ============================================================
FROM python:3.12-slim

WORKDIR /app

# 沙箱服务本体 + 行为审计钩子 (Python audit hook / Node require hook)
COPY sandbox_server.py /app/sandbox_server.py
COPY tcp_proxy.py /app/tcp_proxy.py
COPY hooks/ /opt/sandbox-hooks/

# 工作目录 (运行时由 tmpfs 覆盖, 此处保证只读根下仍有可写挂载点)
RUN mkdir -p /work && chmod 1777 /work

ENV SANDBOX_WORK=/work \
    SANDBOX_HOOKS=/opt/sandbox-hooks \
    SANDBOX_PORT=8090 \
    PYTHONUNBUFFERED=1

EXPOSE 8090

HEALTHCHECK --interval=10s --timeout=5s --retries=5 \
  CMD python -c "import urllib.request,json;r=urllib.request.urlopen('http://localhost:8090/health',timeout=3);assert json.load(r)['status']=='ok'"

CMD ["python", "/app/sandbox_server.py"]
