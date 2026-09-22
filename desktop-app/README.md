# SkillHub 桌面客户端 (Electron)

把 SkillHub 平台打包成 Windows 桌面 App，支持随时切换「本地版 / 公网网页版 / 自定义入口」。

## 运行

```powershell
cd desktop-app
npm install          # 首次安装 Electron
npm start            # 启动桌面客户端
```

## 菜单

- **SkillHub → 后端地址设置… (Ctrl+,)**：切换到本地版 / 公网版 / 自定义地址
- **地址**：一键切本地版（localhost:4173）或公网网页版（GitHub Pages）
- **视图**：缩放 / 全屏 / 开发者工具
- **帮助 → 关于**：查看当前入口与 Electron/Chromium/Node 版本

## 打包成 exe（可选）

```powershell
npm run dist
```

产物在 `desktop-app/release/`（NSIS 安装包 + 免安装 portable）。

## 说明

- 桌面壳只负责「加载 SkillHub 网页 + 地址切换」，不复制业务逻辑；网页更新后客户端自动跟着更新。
- 页面里的外部链接会用系统浏览器打开。
- 加载失败会显示一个可重试的错误页。
