const token = () => localStorage.getItem('ufi-device-token') || '';

async function request(path, options = {}) {
  const headers = {...(options.headers || {})};
  if (token()) headers['X-Device-Token'] = token();
  if (options.json !== undefined) {
    headers['Content-Type'] = 'application/json';
    options.body = JSON.stringify(options.json);
  }
  const response = await fetch(path, {...options, headers});
  const text = await response.text();
  let data;
  try { data = JSON.parse(text); } catch { data = {message: text}; }
  if (!response.ok) throw new Error(data.message || `HTTP ${response.status}`);
  return data;
}

function toast(message) {
  const element = document.querySelector('#toast');
  element.textContent = message;
  element.classList.add('show');
  setTimeout(() => element.classList.remove('show'), 2600);
}

async function ensureToken() {
  const state = await fetch('/api/need_token').then(response => response.json());
  if (state.need_token && !token()) {
    const value = window.prompt('请输入设备令牌（首次启动可从 data/device-token 获取）');
    if (value) localStorage.setItem('ufi-device-token', value.trim());
  }
}

async function refreshDevice() {
  const response = await request('/api/device/status');
  document.querySelector('#device').textContent = response.data.model;
  document.querySelector('#network').textContent = `${response.data.network_type} · 信号 ${response.data.signal}%`;
  document.querySelector('#deviceJson').textContent = JSON.stringify(response.data, null, 2);
}

document.querySelectorAll('[data-tab]').forEach(button => button.addEventListener('click', () => {
  document.querySelectorAll('[data-tab],.panel').forEach(element => element.classList.remove('active'));
  button.classList.add('active');
  document.querySelector(`#${button.dataset.tab}`).classList.add('active');
}));

document.querySelector('#refreshDevice').onclick = () => refreshDevice().catch(error => toast(error.message));
document.querySelector('#startTTYD').onclick = () => request('/api/ttyd/start', {method:'POST'}).then(() => toast('ttyd 已启动')).catch(error => toast(error.message));
document.querySelector('#stopTTYD').onclick = () => request('/api/ttyd/stop', {method:'POST'}).then(() => toast('ttyd 已停止')).catch(error => toast(error.message));
document.querySelector('#enableADB').onclick = () => request('/api/adb/mode', {method:'POST',json:{enabled:true,port:5555}}).then(() => toast('网络 ADB 已启用')).catch(error => toast(error.message));
document.querySelector('#disableADB').onclick = () => request('/api/adb/mode', {method:'POST',json:{enabled:false,port:5555}}).then(() => toast('网络 ADB 已关闭')).catch(error => toast(error.message));

document.querySelector('#loadHosts').onclick = () => request('/api/hosts').then(response => { document.querySelector('#hostsContent').value = response.data.content || ''; }).catch(error => toast(error.message));
document.querySelector('#saveHosts').onclick = () => request('/api/hosts', {method:'POST',json:{content:document.querySelector('#hostsContent').value}}).then(() => toast('Hosts 已保存')).catch(error => toast(error.message));
document.querySelector('#restoreHosts').onclick = () => request('/api/hosts/restore', {method:'POST'}).then(() => toast('Hosts 已恢复')).catch(error => toast(error.message));

document.querySelector('#loadCatalog').onclick = () => request('/api/store/plugins').then(response => {
  const list = document.querySelector('#pluginList'); list.textContent = '';
  (response.data.plugins || []).forEach(plugin => {
    const row = document.createElement('div'); row.className = 'list-item';
    const label = document.createElement('span'); label.textContent = `${plugin.name} ${plugin.version}`;
    const install = document.createElement('button'); install.textContent = '安装';
    install.onclick = () => request('/api/plugin/install',{method:'POST',json:{id:plugin.id}}).then(() => toast(`${plugin.name} 已安装`)).catch(error => toast(error.message));
    row.append(label, install); list.append(row);
  });
}).catch(error => toast(error.message));
document.querySelector('#loadInstalled').onclick = () => request('/api/plugin/list').then(response => {
  document.querySelector('#pluginList').textContent = JSON.stringify(response.data, null, 2);
}).catch(error => toast(error.message));

document.querySelector('#saveTask').onclick = () => {
  let data; try { data = JSON.parse(document.querySelector('#taskData').value); } catch { return toast('任务 JSON 无效'); }
  request('/api/scheduler',{method:'POST',json:{id:document.querySelector('#taskId').value,data}}).then(() => toast('任务已保存')).catch(error => toast(error.message));
};
document.querySelector('#loadTasks').onclick = () => request('/api/scheduler').then(response => { document.querySelector('#taskList').textContent = JSON.stringify(response.data,null,2); }).catch(error => toast(error.message));

document.querySelector('#saveSMS').onclick = async () => {
  let config; try { config = JSON.parse(document.querySelector('#smsConfig').value || '{}'); } catch { return toast('转发配置 JSON 无效'); }
  const method = document.querySelector('#smsMethod').value;
  try {
    await request('/api/sms_forward_enabled',{method:'POST',json:{enabled:document.querySelector('#smsEnabled').checked}});
    await request('/api/sms_forward_method',{method:'POST',json:{method}});
    const endpoint = {CURL:'curl',DINGTALK:'dingtalk',SMTP:'mail',SMS:'sms'}[method];
    await request(`/api/sms_forward_${endpoint}`,{method:'POST',json:config});
    toast('短信转发配置已保存');
  } catch (error) { toast(error.message); }
};

ensureToken().then(async () => {
  const version = await request('/api/version');
  document.querySelector('#service').textContent = '运行中';
  document.querySelector('#version').textContent = version.data.version;
  await refreshDevice();
}).catch(error => { document.querySelector('#service').textContent = '不可用'; toast(error.message); });
