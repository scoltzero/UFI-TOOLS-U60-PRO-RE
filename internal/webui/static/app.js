async function getJSON(path) {
  const token = localStorage.getItem('ufi-device-token') || '';
  const response = await fetch(path, {headers: token ? {'X-Device-Token': token} : {}});
  if (!response.ok) throw new Error(String(response.status));
  return response.json();
}

async function ensureToken() {
  const state = await fetch('/api/need_token').then(response => response.json());
  if (state.need_token && !localStorage.getItem('ufi-device-token')) {
    const token = window.prompt('请输入设备令牌（首次启动可从 data/device-token 获取）');
    if (token) localStorage.setItem('ufi-device-token', token.trim());
  }
}

getJSON('/api/version').then(({data}) => {
  document.querySelector('#service').textContent = '运行中';
  document.querySelector('#version').textContent = data.version;
}).catch(() => document.querySelector('#service').textContent = '不可用');

ensureToken().then(() => getJSON('/api/device/status')).then(({data}) => {
  document.querySelector('#device').textContent = data.model;
  document.querySelector('#network').textContent = `${data.network_type} · 信号 ${data.signal}%`;
}).catch(() => {});
