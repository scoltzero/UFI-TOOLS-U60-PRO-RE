// SPDX-License-Identifier: AGPL-3.0-or-later
// Execute current manager functions in a temp filesystem, using real datad and
// real processes. The DRM renderer and vendor service are explicitly stand-ins.
const assert=require('node:assert/strict');
const fs=require('node:fs/promises');
const path=require('node:path');
const os=require('node:os');
const vm=require('node:vm');
const net=require('node:net');
const {spawn,execFile}=require('node:child_process');
const {promisify}=require('node:util');
const exec=promisify(execFile);
const repo=path.resolve(__dirname,'..');
const workspace=process.env.UFI_PLUGIN_WORKSPACE||path.resolve(repo,'../../ufitools_plugins');
const wait=ms=>new Promise(r=>setTimeout(r,ms));

(async()=>{
 const root=await fs.mkdtemp(path.join(os.tmpdir(),'ufi-plugin-runtime-'));
 const map=p=>path.join(root,p.replace(/^\//,''));
 const foreign=[];
 let functions;let procCommands=0;let maxCommandBytes=0;
 const run=async(command)=>{try{return {success:true,content:(await exec('/bin/sh',['-c',command],{env,timeout:20000,maxBuffer:1<<20})).stdout}}catch(e){return {success:false,content:String(e.stdout||'')+String(e.stderr||'')}}};
 const env={...process.env,PATH:map('/fake-bin')+':'+path.join(workspace,'core/datad-src/tests/fake-bin')+':'+process.env.PATH,
  UFI_FIXTURE_ROOT:root,UBUS_LOG:map('/ubus.log'),UBUS_SIM_FILE:map('/sim.json'),UBUS_TRAFFIC_FILE:map('/traffic.json'),
  ZWRT_DATAD_DATA_DIR:map('/data/plugins/zwrt-datad/state'),ZWRT_DATAD_SYSTEM_TIMEZONE_FILE:map('/etc/config/zwrt_zte_sntp')};
 const tracked=async()=>{
  const listing=(await exec('/bin/ps',['-axo','pid=,comm='])).stdout;
  return listing.split('\n').map(line=>line.trim().match(/^(\d+)\s+(.+)$/)).filter(Boolean).filter(m=>m[2].startsWith(root+'/')).map(m=>({pid:Number(m[1]),exe:m[2]}));
 };
 try{
  for(const p of ['/fake-bin','/data/plugins/u60pro-devui/ui/functions','/data/plugins/zwrt-datad/state','/etc/config','/etc/init.d','/etc/rc.d','/tmp','/proc','/foreign'])await fs.mkdir(map(p),{recursive:true});
  const executable=async(p,body)=>{await fs.writeFile(map(p),body,{mode:0o700})};
  await executable('/fake-bin/pidof',`#!/usr/bin/env node\nconst {execFileSync}=require('node:child_process');const path=require('node:path');const root=process.env.UFI_FIXTURE_ROOT;const names=process.argv.slice(2);const found=execFileSync('/bin/ps',['-axo','pid=,comm='],{encoding:'utf8'}).split('\\n').map(x=>x.trim().match(/^(\\d+)\\s+(.+)$/)).filter(Boolean).filter(m=>m[2].startsWith(root+'/')&&names.includes(path.basename(m[2]))).map(m=>m[1]);if(found.length)console.log(found.join(' '));else process.exitCode=1;\n`);
  await executable('/fake-bin/readlink',`#!/usr/bin/env node\nconst {execFileSync}=require('node:child_process');const args=process.argv.slice(2);const m=(args[0]||'').match(/^\\/proc\\/(\\d+)\\/exe$/);try{if(m){const p=execFileSync('/bin/ps',['-p',m[1],'-o','comm='],{encoding:'utf8'}).trim();if(!p.startsWith(process.env.UFI_FIXTURE_ROOT+'/'))process.exit(1);console.log(require('node:fs').realpathSync(p))}else process.stdout.write(execFileSync('/usr/bin/readlink',args))}catch{process.exitCode=1}\n`);
  await executable('/fake-bin/killall',`#!/bin/sh\nprintf '%s\\n' "$*" >> '${map('/killall.log')}'\n[ "$*" = '-9 zte_topsw_devui' ] || exit 97\n`);
  await executable('/etc/init.d/zte_topsw_devui',`#!/bin/sh\nprintf '%s\\n' "$*" >> '${map('/vendor.log')}'\n`);
  // ps/readlink and vendor-service adapters are host portability fixtures only.
  await fs.writeFile(map('/etc/config/zwrt_zte_mc_tmp'),"config state 'state'\n option mode_main_state 'mode_power_on'\n");
  await fs.writeFile(map('/proc/cmdline'),'silent_boot.mode=nonsilent\n');
  await fs.writeFile(map('/sim.json'),JSON.stringify({sim_iccid:'8986000000000012345',current_sim_slot:'1',sim_states:'ready',Operator:'CUCC'}));
  await fs.writeFile(map('/traffic.json'),JSON.stringify({real_rx_bytes:1000,real_tx_bytes:2000,real_rx_speed:0,real_tx_speed:0,real_time:1}));
  const c=map('/renderer.c');await fs.writeFile(c,'#include <unistd.h>\nint main(void){for(;;)pause();}\n');
  await exec('cc',[c,'-o',map('/data/plugins/u60pro-devui/u60pro-devui')]);
  const datad=path.join(workspace,'core/datad-src');
  const sources=(await fs.readdir(path.join(datad,'src'))).filter(n=>n.endsWith('.c')).map(n=>path.join(datad,'src',n));
  await exec('cc',['-std=c11','-D_GNU_SOURCE','-O0','-I'+path.join(datad,'include'),...sources,'-o',map('/data/plugins/zwrt-datad/zwrt-datad')]);
  for(const name of ['u60pro-devui','zwrt-datad']){
   const bin=map('/foreign/'+name);await exec('cc',[c,'-o',bin]);
   const proc=spawn(bin,[],{stdio:'ignore'});foreign.push(proc);
  }
  const listener=net.createServer();await new Promise(r=>listener.listen(0,'127.0.0.1',r));const port=listener.address().port;await new Promise(r=>listener.close(r));
  let source=await fs.readFile(path.join(workspace,'plugins/u60屏幕管理插件.txt'),'utf8');
  const start=source.indexOf('    // === 路径常量 ===');const end=source.indexOf('    // ====================== UI',start);assert(start>0&&end>start);
  source=source.slice(start,end);
  // Relocate fixed roots and the listen port; execute the actual manager functions.
  for(const dir of ['/data/','/etc/','/tmp/','/sys/','/dev/input/','/proc/cmdline'])source=source.replaceAll(dir,map(dir));
  source=source.replace('-i 1000 --auth-token-file',`-i 100 -p ${port} --auth-token-file`);
  const values=new Map();
  const context={URL,URLSearchParams,Map,console,window:{},UFI_DATA:{},wait,waitForBody:async()=>{},
   localStorage:{getItem:k=>values.get(k)||null,setItem:(k,v)=>values.set(k,v)},
   LOG:()=>{},createToast:()=>{},runShellWithRoot:async command=>{procCommands++;maxCommandBytes=Math.max(maxCommandBytes,Buffer.byteLength(command));assert(maxCommandBytes<=65536);return run(command)}};
  functions=await vm.runInNewContext(`(async()=>{${source}\nreturn {startOurServices,stopOurServices,verifyRunning,setBoot,fullUninstall,installSupportFiles,replaceComponentBinary};})()`,context);
  // Allow the actual initial directory setup to finish before writing fixture files.
  await functions.installSupportFiles();
  const baseline='#!/bin/sh\n# user u60pro_devui custom marker must remain\necho user-task\nexit 0\n';await fs.writeFile(map('/etc/rc.local'),baseline);
  await fs.writeFile(map('/data/plugins/u60pro-devui/ui/.devui-managed-files'),'01-signal.html\nfunctions/managed.html\n');
  await fs.writeFile(map('/data/plugins/u60pro-devui/ui/01-signal.html'),'managed page');
  await fs.writeFile(map('/data/plugins/u60pro-devui/ui/functions/managed.html'),'managed function');
  const custom=map('/data/plugins/u60pro-devui/ui/functions/my-custom.html');await fs.writeFile(custom,'USER CUSTOM CONTENT');
  const saved=map('/data/plugins/zwrt-datad/state/user-settings.json');await fs.writeFile(saved,'USER SAVED STATE');
  await Promise.all([functions.startOurServices(),functions.startOurServices()]);assert.equal(await functions.verifyRunning(),true);
  const owned=()=>tracked().then(rows=>rows.filter(r=>r.exe.includes('/data/plugins/')).map(r=>r.pid).sort());
  const first=await owned();assert.equal(first.length,2);
  await functions.startOurServices();assert.deepEqual(await owned(),first,'repeat start replaced or duplicated processes');
  const oldDatad=(await tracked()).find(p=>p.exe.endsWith('/data/plugins/zwrt-datad/zwrt-datad')).pid;
  const oldRenderer=(await tracked()).find(p=>p.exe.endsWith('/data/plugins/u60pro-devui/u60pro-devui')).pid;
  const staged=map('/data/plugins/zwrt-datad/zwrt-datad.new');await fs.copyFile(map('/data/plugins/zwrt-datad/zwrt-datad'),staged);
  await functions.replaceComponentBinary('datad',staged);await functions.startOurServices();
  const updated=await tracked();assert.notEqual(updated.find(p=>p.exe.endsWith('/data/plugins/zwrt-datad/zwrt-datad')).pid,oldDatad);
  assert.equal(updated.find(p=>p.exe.endsWith('/data/plugins/u60pro-devui/u60pro-devui')).pid,oldRenderer,'updating datad restarted renderer');
  const response=await fetch(`http://127.0.0.1:${port}/state`);assert.equal(response.status,200);assert.equal((await response.json()).sim_traffic.available,true);
  await functions.setBoot(true);await functions.setBoot(true);
  const boot=await fs.readFile(map('/etc/rc.local'),'utf8');
  assert.equal(boot.split('\n').filter(l=>l.endsWith('# u60pro_devui_datad')).length,1);
  assert.equal(boot.split('\n').filter(l=>l.endsWith('# u60pro_devui_devui')).length,1);
  assert(boot.includes('# user u60pro_devui custom marker must remain'));
  await functions.stopOurServices();assert.equal(await functions.verifyRunning(),false);assert.equal((await owned()).length,0);
  // Run the actual registered boot lines in the fixture, without rebooting a machine.
  const lines=boot.split('\n').filter(l=>l.endsWith('# u60pro_devui_datad')||l.endsWith('# u60pro_devui_devui'));
  assert((await run(lines.join('\n'))).success);
  for(let i=0;i<40&&!(await functions.verifyRunning());i++)await wait(100);
  assert.equal(await functions.verifyRunning(),true,'registered startup did not launch components');
  // Existing legacy user content is not owned by the current installer.
  await fs.mkdir(map('/data/ui'),{recursive:true});await fs.writeFile(map('/data/ui/user.html'),'LEGACY USER PAGE');
  const outside=map('/foreign/user-page.html');await fs.writeFile(outside,'OUTSIDE USER CONTENT');
  await fs.symlink(map('/foreign'),map('/data/plugins/u60pro-devui/ui/functions/shared'));
  await fs.writeFile(map('/data/plugins/u60pro-devui/ui/.devui-managed-files'),'functions/shared/user-page.html\n');
  await assert.rejects(functions.fullUninstall(),/outside component/);
  assert.equal(await fs.readFile(outside,'utf8'),'OUTSIDE USER CONTENT');
  await fs.writeFile(map('/data/plugins/u60pro-devui/ui/.devui-managed-files'),'01-signal.html\nfunctions/managed.html\n');
  await functions.fullUninstall();assert.equal((await owned()).length,0);
  assert.equal(await fs.readFile(custom,'utf8'),'USER CUSTOM CONTENT');assert.equal(await fs.readFile(saved,'utf8'),'USER SAVED STATE');
  assert.equal(await fs.readFile(map('/data/ui/user.html'),'utf8'),'LEGACY USER PAGE');
  assert.equal(await fs.readFile(map('/etc/rc.local'),'utf8'),baseline);
  for(const p of ['/data/plugins/u60pro-devui/u60pro-devui','/data/plugins/zwrt-datad/zwrt-datad','/data/plugins/u60pro-devui/ui/01-signal.html'])await assert.rejects(fs.stat(map(p)),{code:'ENOENT'});
  for(const proc of foreign){assert.equal(proc.exitCode,null);process.kill(proc.pid,0)}
  assert((await fs.readFile(map('/killall.log'),'utf8')).split('\n').filter(Boolean).every(l=>l==='-9 zte_topsw_devui'));
  console.log(JSON.stringify({realDatad:'PASS',renderer:'stand-in (no DRM)',startStop:'PASS',concurrentStart:'PASS',repeatStartSamePIDs:'PASS',replaceOnlyTarget:'PASS',registeredStartup:'PASS (no reboot)',uninstallManagedFiles:'PASS',customAndSavedState:'preserved',foreignSameNameProcesses:'preserved',symlinkEscape:'rejected; outside file preserved',shellCommands:procCommands,maxCommandBytes}));
 }catch(error){
  for(const file of ['/tmp/zwrt-datad-boot.log','/tmp/u60pro-devui-boot.log']){try{console.error(file,await fs.readFile(map(file),'utf8'))}catch{}}
  console.error('fixture processes',await tracked());throw error;
 }finally{
  for(const proc of await tracked()){try{process.kill(proc.pid,'SIGKILL')}catch{}}
  for(const proc of foreign)await new Promise(r=>proc.exitCode===null?proc.once('exit',r):r());
  await fs.rm(root,{recursive:true,force:true});
 }
})().catch(e=>{console.error(e);process.exitCode=1});
