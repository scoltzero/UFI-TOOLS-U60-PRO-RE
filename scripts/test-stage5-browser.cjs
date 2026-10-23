// SPDX-License-Identifier: AGPL-3.0-or-later
// Invoked by TestStage5BrowserIntegration, against its isolated HTTP handler.
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const path = require('node:path');
const {createRequire} = require('node:module');
const root = path.resolve(__dirname, '..');
const workspace = process.env.UFI_PLUGIN_WORKSPACE || path.resolve(root, '../../ufitools_plugins');
const {chromium} = createRequire(path.join(workspace, 'package.json'))('playwright');
const [base, mode] = process.argv.slice(2);
const token = 'stage5-local-fixture-token';

(async () => {
 const plugin = await fs.readFile(path.join(workspace, 'plugins/u60屏幕管理插件.txt'), 'utf8');
 const marker = '<script>window.__stage5Loads=(window.__stage5Loads||0)+1;</script>';
 const source = marker + '\n' + plugin;
 const api = async (endpoint, body) => {
  const response = await fetch(base + '/api/' + endpoint, {method:body === undefined?'GET':'POST', headers:{authorization:'Bearer '+token,'content-type':'application/json'},body:body===undefined?undefined:JSON.stringify(body)});
  assert.equal(response.status,200,endpoint);return response.json();
 };
 let installedSource=source;
 if(mode==='enabled'){
  await api('set_custom_head',{text:marker});
  await api('store/plugins/screen-manager/install',{});
  await api('plugin/install',{id:'screen-manager'});
  installedSource=(await api('get_custom_head')).text;
  assert.equal(installedSource.split('[KANO_PLUGIN_START] ufi-store:screen-manager').length-1,1);
  assert.equal((await api('plugin/list')).data[0].enabled,true);
 }else await api('set_custom_head',{text:source});
 assert.equal((await api('get_custom_head')).text,installedSource);
 const browser = await chromium.launch({channel:'chrome',headless:true});
 try {
  const context = await browser.newContext({locale:'zh-CN',viewport:{width:1440,height:1000},serviceWorkers:'block'});
  const page = await context.newPage();page.setDefaultTimeout(10000);const errors=[];const external=[];const commands=[];
  const ready=page.waitForEvent('console',{predicate:message=>message.text().includes('[U60DevUI] loaded'),timeout:15000});
  page.on('pageerror',e=>errors.push(e.message));
  page.on('request',r=>{if(r.url().includes('/root_shell'))commands.push(JSON.parse(r.postData()||'{}').command)});
  await page.route('**/*',r=>{const url=new URL(r.request().url());if(url.origin===base)return r.continue();external.push(url.origin);return r.abort()});
  await page.goto(base);await page.locator('#TOKEN').waitFor({state:'visible'});
  assert.equal(await page.evaluate(()=>window.__stage5Loads||0),0);
  await page.locator('#TOKEN').fill('wrong-fixture-token');await page.locator('#tokenModal button').first().click();
  await page.waitForFunction(()=>document.querySelector('#tokenModal').style.display!=='none');
  // Wait for the rejected config request before attempting the correct login.
  await page.waitForTimeout(500);assert.equal(await page.evaluate(()=>window.__stage5Loads||0),0);
  await page.locator('#TOKEN').fill(token);await page.locator('#tokenModal button').first().click();
  await page.waitForFunction(()=>window.__stage5Loads===1);
  await page.locator('#tokenModal').waitFor({state:'hidden'});
  await page.waitForFunction(()=>document.querySelector('#STATUS').innerText.includes('模拟网络'));
  await ready;
  await page.getByRole('button',{name:'屏幕管理',exact:true}).waitFor({state:'attached'});
  await page.evaluate(()=>loadCustomHead());assert.equal(await page.evaluate(()=>window.__stage5Loads),1);
  // Existing control helpers must normalize user_shell output and HTTP errors.
  const result=await page.evaluate(()=>runShellWithUser('id -u'));
  assert.equal(result.success,mode==='enabled');
  assert.equal(typeof result.content,'string');
  if(mode==='enabled')assert.equal(result.content.trim(),'0');
  // Opening the real manager exercises its actual status command, not a copied UI.
  await page.getByRole('button',{name:'屏幕管理',exact:true}).evaluate(button=>button.click());
  await page.locator('#U60DevUIModal_plugin').waitFor({state:'visible'});
  if(mode==='enabled'){
   await page.waitForFunction(()=>document.querySelector('#u60_state')?.innerText.includes('运行中'));
   assert.equal(await page.locator('#u60_state').getByText('运行中',{exact:true}).count(),2);
   assert(commands.some(c=>c.includes('INSTALLED_DEVUI=')));
   assert.equal(await page.locator('#u60_install').innerText(),'组件已安装（版本未核对）');
  }else{
   const failed=await page.evaluate(()=>runShellWithRoot('id -u'));
   assert.equal(failed.success,false);assert.match(failed.content,/disabled/i);
   await page.waitForFunction(()=>document.querySelector('#u60_state')?.innerText.includes('状态读取失败'));
   assert.equal(await page.locator('#u60_state').getByText('未安装',{exact:true}).count(),0);
  }
  const output=path.join(root,'output/playwright');await fs.mkdir(output,{recursive:true});
  await page.waitForFunction(()=>getComputedStyle(document.querySelector('#U60DevUIModal_plugin')).opacity==='1');
  await page.locator('#U60DevUIModal_plugin').screenshot({path:path.join(output,`stage5-devui-${mode}.png`)});
  await page.reload();await page.waitForFunction(()=>window.__stage5Loads===1);
  assert.equal(await page.getByRole('button',{name:'屏幕管理',exact:true}).count(),1);
  // Also check the modern endpoint used by the manager's fallback shim.
  const shim=await page.evaluate(async()=>{const r=await fetch('/api/run_shell',{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify({cmd:'id -u'})});return {status:r.status,body:await r.json()}});
  assert.equal(shim.status,mode==='enabled'?200:403);
  if(mode==='enabled'){assert.equal(shim.body.success,true);assert.equal(shim.body.content.trim(),'0');assert.equal(shim.body.data.output.trim(),'0')}
  if(mode==='enabled'){
   const disabled=installedSource.replace(plugin,'<!-- [kano_disabled]\n'+plugin+'\n[kano_disabled] -->');
   await api('set_custom_head',{text:disabled});
   assert.equal((await api('plugin/list')).data[0].enabled,false);
   // Updating the catalog entry must preserve the user's disabled choice.
   await api('plugin/install',{id:'screen-manager'});
   assert.equal((await api('plugin/list')).data[0].enabled,false);
   await page.reload();await page.waitForFunction(()=>window.__stage5Loads===1);
   assert.equal(await page.getByRole('button',{name:'屏幕管理',exact:true}).count(),0);
   await api('set_custom_head',{text:installedSource});
   await page.reload();await page.waitForFunction(()=>window.__stage5Loads===1);
   await page.getByRole('button',{name:'屏幕管理',exact:true}).waitFor({state:'attached'});
   await api('plugin/uninstall',{id:'screen-manager'});
   assert.equal((await api('plugin/list')).data.length,0);
   assert((await api('get_custom_head')).text.includes(marker));
   await page.reload();await page.waitForFunction(()=>window.__stage5Loads===1);
   assert.equal(await page.getByRole('button',{name:'屏幕管理',exact:true}).count(),0);
  }
  assert.deepEqual(errors,[]);assert.deepEqual(external,[]);
  console.log(JSON.stringify({scenario:mode,catalogLifecycle:mode==='enabled'?'PASS (install/reinstall/disable/update/re-enable/uninstall)':'not configured',login:'PASS',pluginLoadedAfterLogin:'PASS',duplicateLoad:'PASS',reload:'PASS',managerStatus:mode==='enabled'?'PASS (fixture runner)':'blocked by explicit shell gate',shellResponses:'PASS',pageErrors:errors.length,externalRequests:external.length,maxCommandBytes:Math.max(0,...commands.map(c=>Buffer.byteLength(c)))}));
 } finally {await browser.close()}
})().catch(error=>{console.error(error);process.exitCode=1});
