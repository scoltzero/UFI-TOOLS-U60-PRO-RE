async function getJSON(path) {
  const token = localStorage.getItem('ufi-device-token') || '';
  const response = await fetch(path, {headers: token ? {'X-Device-Token': token} : {}});
  if (!response.ok) throw new Error(String(response.status));
  return response.json();
}

getJSON('/api/version').then(({data}) => {
  document.querySelector('#service').textContent = '运行中';
  document.querySelector('#version').textContent = data.version;
}).catch(() => document.querySelector('#service').textContent = '不可用');

getJSON('/api/device/status').then(({data}) => {
  document.querySelector('#device').textContent = data.model;
  document.querySelector('#network').textContent = `${data.network_type} · 信号 ${data.signal}%`;
}).catch(() => {});

