const token = () => sessionStorage.getItem('ufi_session_token') || localStorage.getItem('ufi_remembered_token') || '';

async function request(path, options = {}) {
  const headers = {...(options.headers || {})};
  if (token()) headers['X-Device-Token'] = token();
  if (options.json !== undefined) {
    headers['Content-Type'] = 'application/json';
    options.body = JSON.stringify(options.json);
  }
  const response = await fetch(path, {...options, headers});
  if (response.status === 401) {
    window.location.replace('/');
    throw new Error('请先在首页登录');
  }
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
    window.location.replace('/');
    return false;
  }
  return true;
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
  if (button.dataset.tab === 'system-panel') loadRuntimeSettings().catch(error => toast(error.message));
}));

document.querySelector('#refreshDevice').onclick = () => refreshDevice().catch(error => toast(error.message));
document.querySelector('#startTTYD').onclick = () => request('/api/ttyd/start', {method:'POST'}).then(() => toast('ttyd 已启动')).catch(error => toast(error.message));
document.querySelector('#stopTTYD').onclick = () => request('/api/ttyd/stop', {method:'POST'}).then(() => toast('ttyd 已停止')).catch(error => toast(error.message));
document.querySelector('#enableADB').onclick = () => request('/api/adb/mode', {method:'POST',json:{enabled:true,port:5555}}).then(() => toast('网络 ADB 已启用')).catch(error => toast(error.message));
document.querySelector('#disableADB').onclick = () => request('/api/adb/mode', {method:'POST',json:{enabled:false,port:5555}}).then(() => toast('网络 ADB 已关闭')).catch(error => toast(error.message));

document.querySelector('#loadHosts').onclick = () => request('/api/hosts').then(response => { document.querySelector('#hostsContent').value = response.data.content || ''; }).catch(error => toast(error.message));
document.querySelector('#saveHosts').onclick = () => request('/api/hosts', {method:'POST',json:{content:document.querySelector('#hostsContent').value}}).then(response => toast(response.data.applied ? 'Hosts 已应用' : 'Hosts 草稿已保存，尚未配置系统文件路径')).catch(error => toast(error.message));
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
  request('/api/scheduler',{method:'POST',json:{id:document.querySelector('#taskId').value,data:{...data,next_unix:data.next_unix || Math.floor(Date.now()/1000)+60}}}).then(() => toast('任务已保存')).catch(error => toast(error.message));
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

ensureToken().then(async authenticated => {
  if (!authenticated) return;
  const version = await request('/api/version');
  document.querySelector('#service').textContent = '运行中';
  document.querySelector('#version').textContent = version.data.version;
  await refreshDevice();
}).catch(error => { document.querySelector('#service').textContent = '不可用'; toast(error.message); });

const report = fn => async () => {try {await fn()} catch(error) {toast(error.message)}};
document.querySelector('#readRuntime').onclick=report(async()=>{const result={};for(const path of ['capabilities','usb_status','time/config','auto_update/config','log_clear'])result[path]=await request('/api/'+path);document.querySelector('#runtimeState').textContent=JSON.stringify(result,null,2)});
function showRuntimeSettings(data) {
  const {active, runtime, ...settings} = data;
  document.querySelector('#runtimeConfig').value = JSON.stringify(settings, null, 2);
  document.querySelector('#runtimeState').textContent = JSON.stringify(data, null, 2);
}
async function loadRuntimeSettings() {
  const job = document.querySelector('#runtimeJob').value;
  document.querySelector('#saveRuntime').disabled = true;
  const result = await request('/api/' + job);
  if (job !== document.querySelector('#runtimeJob').value) return;
  showRuntimeSettings(result.data);
  document.querySelector('#saveRuntime').disabled = false;
}
document.querySelector('#loadRuntime').onclick = report(loadRuntimeSettings);
document.querySelector('#runtimeJob').onchange = report(loadRuntimeSettings);
document.querySelector('#saveRuntime').onclick=report(async()=>{const job=document.querySelector('#runtimeJob').value;const result=await request('/api/'+job,{method:'POST',json:JSON.parse(document.querySelector('#runtimeConfig').value)});if(job===document.querySelector('#runtimeJob').value)showRuntimeSettings(result.data);toast(result.data.active?'设置已保存，后台已启用':'设置已保存，当前后台未启用')});
document.querySelector('#syncLocalTime').onclick=report(async()=>{await request('/api/time/sync',{method:'POST',json:{unix:Math.floor(Date.now()/1000)}});toast('设备时间已同步')});
document.querySelector('#readLogs').onclick=report(async()=>{const res=await request('/api/logs');document.querySelector('#candidateLogs').textContent=res.message||''});
document.querySelector('#clearLogs').onclick=report(async()=>{await request('/api/logs',{method:'DELETE'});document.querySelector('#candidateLogs').textContent='';toast('候选日志已清空')});
document.querySelector('#uploadCandidateFile').onclick=report(async()=>{const file=document.querySelector('#candidateFile').files[0];if(!file)throw new Error('请选择文件');const form=new FormData();form.append('file',file);await request('/api/upload_file',{method:'POST',body:form});toast('文件已保存至候选存储')});
document.querySelector('#listCandidateFiles').onclick=report(async()=>{const result=await request('/api/uploads');const list=document.querySelector('#candidateFiles');list.replaceChildren();for(const item of result.data||[]){const button=document.createElement('button');button.textContent=`下载 ${item.name} (${item.size} B)`;button.onclick=report(async()=>{const response=await fetch('/api/uploads/'+encodeURIComponent(item.name),{headers:{'X-Device-Token':token()}});if(!response.ok)throw new Error('下载失败');const url=URL.createObjectURL(await response.blob());const a=document.createElement('a');a.href=url;a.download=item.name;a.click();setTimeout(()=>URL.revokeObjectURL(url),1000)});list.append(button)}});
document.querySelector('#calibrateUsage').onclick=report(async()=>{const value=document.querySelector('#calibratedBytes').value;const result=await request('/api/goform/goform_set_cmd_process',{method:'POST',json:{goformId:'FLOW_CALIBRATION_MANUAL',data:value}});toast(result.result==='success'?'本月用量已校准':'校准失败')});
for(const [id,path,method]of [['checkSignedUpdate','check_update','GET'],['downloadSignedUpdate','download_update','POST'],['installSignedUpdate','install_update','POST']])document.querySelector('#'+id).onclick=report(async()=>{const result=await request('/api/'+path,{method});document.querySelector('#signedUpdateState').textContent=JSON.stringify(result,null,2)});

document.querySelector('#setFirmwareUSBMode').onclick=report(async()=>{await request('/api/usb/mode',{method:'POST',json:{mode:document.querySelector('#firmwareUSBMode').value.trim()}});toast('USB 模式已应用，可能需要重新连接设备')});
