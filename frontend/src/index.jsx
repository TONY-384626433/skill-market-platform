// ============================================================
// 应用入口
// ============================================================
import React from 'react';
import ReactDOM from 'react-dom/client';
import '@fontsource-variable/inter/wght.css';
import '@fontsource-variable/noto-sans-sc/wght.css';
import '@fontsource-variable/jetbrains-mono/wght.css';
import App from './App';
import './index.css';

const root = ReactDOM.createRoot(document.getElementById('root'));
root.render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
