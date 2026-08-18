# ============================================================
# SKILL RUNNER — 私域 Skill 市场技能运行服务镜像
# 内置 4 个 MCP 技能: db-inspection / log-desensitization
#                    alert-convergence / requirement-analysis
# 构建: docker build -t skill-runner ../seed-skills
# 或随 docker compose up -d 自动构建
# ============================================================
FROM python:3.12-slim

WORKDIR /app

# 复制技能代码
COPY db-inspection/ db-inspection/
COPY log-desensitization/ log-desensitization/
COPY alert-convergence/ alert-convergence/
COPY requirement-analysis/ requirement-analysis/
COPY skill-runner.py .

# 统一技能服务入口
EXPOSE 8081

HEALTHCHECK --interval=10s --timeout=5s --retries=5 \
  CMD python -c "import urllib.request;urllib.request.urlopen('http://localhost:8081/db-inspection/mcp', data=b'{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/list\"}', timeout=3)"

CMD ["python", "skill-runner.py", "8081"]
