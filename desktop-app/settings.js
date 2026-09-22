// 设置页逻辑 (渲染进程, contextIsolation 下通过 preload 暴露的桥接调用)
const input = document.getElementById('url');
const hint = document.getElementById('hint');

function setHint(text, cls) {
  hint.textContent = text;
  hint.className = 'hint' + (cls ? ' ' + cls : '');
}

async function init() {
  const cfg = await window.skillhubDesktop.getConfig();
  input.value = cfg.url || '';
  window.skillhubDesktop.onConfigChanged((next) => { input.value = next.url; });
  input.focus();
  input.select();
}

document.getElementById('p-local').addEventListener('click', () => { input.value = 'http://localhost:4173'; input.focus(); });
document.getElementById('p-web').addEventListener('click', () => { input.value = 'https://tony-384626433.github.io/skill-market-platform/'; input.focus(); });

document.getElementById('save').addEventListener('click', async () => {
  const result = await window.skillhubDesktop.setURL(input.value);
  if (result && result.ok) {
    setHint('已保存，正在加载…', 'ok');
    setTimeout(() => window.close(), 500);
  } else {
    setHint((result && result.error) || '保存失败', 'err');
  }
});

document.getElementById('cancel').addEventListener('click', () => window.close());
input.addEventListener('keydown', (event) => { if (event.key === 'Enter') document.getElementById('save').click(); });

init();
