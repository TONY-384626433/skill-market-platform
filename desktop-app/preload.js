// 预加载脚本：以最小暴露面桥接主进程能力
const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('skillhubDesktop', {
  isDesktop: true,
  versions: {
    electron: process.versions.electron,
    chrome: process.versions.chrome,
    node: process.versions.node,
  },
  getConfig: () => ipcRenderer.invoke('config:get'),
  setURL: (url) => ipcRenderer.invoke('config:set-url', url),
  onConfigChanged: (handler) => {
    const listener = (event, cfg) => handler(cfg);
    ipcRenderer.on('config:changed', listener);
    return () => ipcRenderer.removeListener('config:changed', listener);
  },
});
