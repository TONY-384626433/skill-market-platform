# SkillHub Java 桌面客户端

用 **Java 21 + Swing** 写的一个 SkillHub 桌面客户端（零第三方依赖，JDK 自带 HTTP/2 客户端即可）。

## 功能

- 连接 SkillHub 后端（默认 `http://localhost:8080/api/v1`，可改）
- 检索两个来源：
  - **GitHub 开源**：名称 / 仓库 / Stars / 许可 / 本机兼容性
  - **企业审核库**：名称 / 分类 / 版本 / 安装量 / 评分
- 双击 GitHub 结果行 → 用系统浏览器打开该仓库
- 顶部状态栏实时显示：连接结果、匹配总数、账号连接状态

## 编译 & 运行

```powershell
cd java-app

# 编译 + 打包（自动定位 JDK：JAVA_HOME → PATH → 常见安装目录）
powershell -ExecutionPolicy Bypass -File build.ps1

# 运行（或直接双击 run.bat）
java -jar dist\SkillHubDesktop.jar
```

## 打成 Windows 软件（jpackage）

```powershell
# 免安装版（app-image，自带精简 JRE）+ .exe 安装包（需 WiX 3.x 的 candle/light 在 PATH）
powershell -ExecutionPolicy Bypass -File package-software.ps1
```

产物：
- `release\SkillHub\SkillHub.exe` —— 免安装独立软件（约 75 MB，含运行时）
- `release-installer\SkillHub-1.0.0.exe` —— 带安装向导的安装包（开始菜单 + 桌面快捷方式，免管理员安装）

> WiX 未装时脚本会自动跳过安装包、只出免安装版。

## 环境要求

- JDK 17+（本机已装 Microsoft OpenJDK 21，`JAVA_HOME` 已配置）
- 打包安装包：WiX Toolset 3.x（`candle.exe` / `light.exe`）

## 目录

```
java-app/
  src/com/skillhub/desktop/
    App.java             # Swing 界面 (主窗口/表格/搜索)
    SkillHubClient.java  # 后端 API 客户端 (java.net.http)
    Json.java            # 极简 JSON 解析器 (零依赖)
  build.ps1              # 编译 + 打 jar
  run.bat                # 启动脚本
```

## 说明

- 与 Electron 桌面版并存：Electron 版是「网页壳」，Java 版是「原生客户端」，都连同一套后端 API。
- `out/`、`dist/` 为构建产物，已 gitignore。
