// SkillHub 桌面客户端 — Electron 主进程
// 作用：把 SkillHub 网页应用包成一个 Windows 桌面 App，可切换本地版/公网版后端。
const { app, BrowserWindow, Menu, shell, ipcMain, dialog, Tray, nativeImage } = require('electron');
const path = require('path');
const fs = require('fs');

const ICON_PATH = path.join(__dirname, 'assets', 'skillhub.ico');
function appIcon() {
  try { return nativeImage.createFromPath(ICON_PATH); } catch { return undefined; }
}

const PRESETS = {
  local: 'http://localhost:4173',
  web: 'https://tony-384626433.github.io/skill-market-platform/',
};

const CONFIG_FILE = () => path.join(app.getPath('userData'), 'skillhub-app.json');

function readConfig() {
  try {
    const raw = JSON.parse(fs.readFileSync(CONFIG_FILE(), 'utf8'));
    return { url: raw.url || PRESETS.local, presets: PRESETS };
  } catch {
    return { url: PRESETS.local, presets: PRESETS };
  }
}

function writeConfig(cfg) {
  try {
    fs.mkdirSync(path.dirname(CONFIG_FILE()), { recursive: true });
    fs.writeFileSync(CONFIG_FILE(), JSON.stringify({ url: cfg.url }, null, 2), 'utf8');
    return true;
  } catch (error) {
    dialog.showErrorBox('保存失败', String(error));
    return false;
  }
}

let config = { url: PRESETS.local, presets: PRESETS };
let mainWindow = null;
let settingsWindow = null;
let tray = null;

function autostartEnabled() {
  try { return app.getLoginItemSettings().openAtLogin; } catch { return false; }
}
function setAutostart(enable) {
  try {
    app.setLoginItemSettings({ openAtLogin: Boolean(enable), path: process.execPath, args: [] });
    if (tray) buildTray();
    return true;
  } catch (error) {
    dialog.showErrorBox('设置开机自启失败', String(error));
    return false;
  }
}

function buildMenu() {
  const template = [
    {
      label: 'SkillHub',
      submenu: [
        { label: '重新加载', accelerator: 'F5', click: () => mainWindow && mainWindow.reload() },
        { label: '强制刷新', accelerator: 'Ctrl+Shift+R', click: () => mainWindow && mainWindow.webContents.reloadIgnoringCache() },
        { type: 'separator' },
        { label: '后端地址设置…', accelerator: 'Ctrl+,', click: openSettings },
        { type: 'separator' },
        { label: '开机自启', type: 'checkbox', checked: autostartEnabled(), click: (item) => setAutostart(item.checked) },
        { type: 'separator' },
        { role: 'quit', label: '退出' },
      ],
    },
    {
      label: '地址',
      submenu: [
        { label: `本地版 (${PRESETS.local})`, click: () => setURL(PRESETS.local) },
        { label: `公网网页版 (GitHub Pages)`, click: () => setURL(PRESETS.web) },
        { type: 'separator' },
        { label: '自定义…', click: openSettings },
      ],
    },
    {
      label: '视图',
      submenu: [
        { role: 'zoomIn', label: '放大' },
        { role: 'zoomOut', label: '缩小' },
        { role: 'resetZoom', label: '重置缩放' },
        { type: 'separator' },
        { role: 'togglefullscreen', label: '全屏' },
        { role: 'toggleDevTools', label: '开发者工具' },
      ],
    },
    {
      label: '帮助',
      submenu: [
        { label: '关于 SkillHub', click: showAbout },
        { label: '打开项目主页', click: () => shell.openExternal('https://github.com/TONY-384626433/skill-market-platform') },
      ],
    },
  ];
  Menu.setApplicationMenu(Menu.buildFromTemplate(template));
}

function setURL(url) {
  if (!url) return;
  config.url = url;
  writeConfig(config);
  if (mainWindow) mainWindow.loadURL(url);
  if (settingsWindow) settingsWindow.webContents.send('config:changed', config);
}

function errorPage(url, code, desc) {
  const html = `<!DOCTYPE html><html lang="zh-CN"><head><meta charset="utf-8"><title>连接失败</title>
  <style>body{margin:0;height:100vh;display:flex;align-items:center;justify-content:center;background:#0d1b2a;color:#e6edf7;font-family:'Segoe UI',system-ui,sans-serif}
  .box{max-width:560px;padding:36px 40px;border:1px solid #1f3a5f;border-radius:14px;background:#12233a;box-shadow:0 12px 40px rgba(0,0,0,.4)}
  h1{margin:0 0 10px;font-size:20px}p{color:#9fb4d0;line-height:1.7;word-break:break-all}
  code{color:#5cc8ff}button{margin-top:18px;padding:10px 20px;border:0;border-radius:8px;background:#0b63ce;color:#fff;cursor:pointer;font-size:14px}
  .u{margin-top:12px;font-size:13px;color:#7f95b3}</style></head>
  <body><div class="box"><h1>⚠ 无法连接 SkillHub</h1>
  <p>目标地址：<code>${url}</code></p>
  <p>错误：${desc} (${code})</p>
  <p class="u">请确认后端/前端已启动，或在菜单「地址」里切换到其它入口。</p>
  <button onclick="location.href='${url}'">重试</button></div></body></html>`;
  return 'data:text/html;charset=utf-8,' + encodeURIComponent(html);
}

function buildTray() {
  if (!tray) {
    tray = new Tray(appIcon() || nativeImage.createEmpty());
    tray.setToolTip('SkillHub · 九江银行 AI 能力中心');
    tray.on('double-click', () => showMainWindow());
  }
  const menu = Menu.buildFromTemplate([
    { label: '显示主窗口', click: showMainWindow },
    { label: '隐藏到后台', click: () => mainWindow && mainWindow.hide() },
    { type: 'separator' },
    { label: '重新加载', click: () => mainWindow && mainWindow.reload() },
    { label: '后端地址设置…', click: openSettings },
    { type: 'separator' },
    { label: '开机自启', type: 'checkbox', checked: autostartEnabled(), click: (item) => setAutostart(item.checked) },
    { type: 'separator' },
    { label: '退出 SkillHub', click: () => { app.isQuitting = true; app.quit(); } },
  ]);
  tray.setContextMenu(menu);
}

function showMainWindow() {
  if (!mainWindow) { createWindow(); return; }
  if (mainWindow.isMinimized()) mainWindow.restore();
  mainWindow.show();
  mainWindow.focus();
}

function openSettings() {
  if (settingsWindow) { settingsWindow.focus(); return; }
  settingsWindow = new BrowserWindow({
    width: 560,
    height: 420,
    resizable: false,
    parent: mainWindow || undefined,
    modal: Boolean(mainWindow),
    title: 'SkillHub · 后端地址设置',
    autoHideMenuBar: true,
    webPreferences: { preload: path.join(__dirname, 'preload.js'), contextIsolation: true, nodeIntegration: false },
  });
  settingsWindow.loadFile(path.join(__dirname, 'settings.html'));
  settingsWindow.on('closed', () => { settingsWindow = null; });
}

function showAbout() {
  dialog.showMessageBox(mainWindow || undefined, {
    type: 'info',
    title: '关于 SkillHub',
    message: 'SkillHub 桌面客户端 v1.0.0',
    detail: `九江银行内部 AI 能力中心 · 桌面版\n\n当前入口：${config.url}\nElectron ${process.versions.electron}\nChromium ${process.versions.chrome}\nNode ${process.versions.node}`,
    buttons: ['好'],
  });
}

function createWindow() {
  mainWindow = new BrowserWindow({
    width: 1360,
    height: 860,
    minWidth: 1024,
    minHeight: 680,
    title: 'SkillHub · 九江银行 AI 能力中心',
    icon: appIcon(),
    backgroundColor: '#0d1b2a',
    autoHideMenuBar: false,
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
      nodeIntegration: false,
    },
  });

  mainWindow.loadURL(config.url);

  mainWindow.webContents.setWindowOpenHandler(({ url }) => {
    // 外部链接用系统浏览器打开
    if (url.startsWith('http')) { shell.openExternal(url); return { action: 'deny' }; }
    return { action: 'allow' };
  });

  mainWindow.webContents.on('did-fail-load', (event, errorCode, errorDescription, validatedURL, isMainFrame) => {
    if (!isMainFrame) return;
    if (errorCode === -3) return; // aborted
    mainWindow.loadURL(errorPage(validatedURL || config.url, errorCode, errorDescription));
  });

  mainWindow.on('close', (event) => {
    // 默认关闭即退出；若托盘存在且非主动退出，则最小化到托盘
    if (tray && !app.isQuitting) {
      event.preventDefault();
      mainWindow.hide();
    }
  });

  mainWindow.on('closed', () => { mainWindow = null; });
}

app.whenReady().then(() => {
  config = readConfig();
  app.setAppUserModelId('com.skillhub.desktop');
  buildMenu();
  createWindow();
  try { buildTray(); } catch (error) { console.warn('托盘创建失败（不影响使用）:', error && error.message); }

  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) createWindow();
  });
});

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') app.quit();
});

ipcMain.handle('config:get', () => config);
ipcMain.handle('app:autostart-get', () => autostartEnabled());
ipcMain.handle('app:autostart-set', (event, enable) => setAutostart(enable));
ipcMain.handle('config:set-url', (event, url) => {
  const next = String(url || '').trim();
  if (!/^https?:\/\//i.test(next)) return { ok: false, error: '地址必须以 http:// 或 https:// 开头' };
  setURL(next);
  return { ok: true, url: next };
});
