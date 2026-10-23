const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');

test('management redirects missing or expired homepage login without a password prompt', async () => {
    const source=fs.readFileSync(path.join(__dirname,'static/app.js'),'utf8');
    const session=storage(),remembered=storage(),redirects=[];
    let status=200;
    const context={sessionStorage:session,localStorage:remembered,window:{location:{replace:url=>redirects.push(url)}},
        fetch:async()=>({status,ok:status===200,json:async()=>({need_token:true}),text:async()=>'{}'})};
    vm.createContext(context);vm.runInContext(source.slice(0,source.indexOf('async function refreshDevice')),context);
    assert.equal(await vm.runInContext('ensureToken()',context),false);
    assert.deepEqual(redirects,['/']);
    session.setItem('ufi_session_token','synthetic-shared-password');
    assert.equal(await vm.runInContext('ensureToken()',context),true);
    status=401;
    await assert.rejects(vm.runInContext("request('/api/capabilities')",context),/请先在首页登录/);
    assert.deepEqual(redirects,['/','/']);
});

test('eight-character password change shares the session without displaying the password', async () => {
    const source=fs.readFileSync(path.join(__dirname,'static/script/main.js'),'utf8');
    const start=source.indexOf('    const handleChangeToken = async (e) => {');
    const end=source.indexOf('    const onCloseChangeTokenForm',start);
    const saved=[],toasts=[],closed=[],posts=[];let reset=false;
    const context={FormData:class {get(){return 'Lab12345'}},localStorage:storage(),KANO_baseURL:'/api',common_headers:{},
        createToast:(...args)=>toasts.push(args),t:key=>key,storeLoginToken:(...args)=>saved.push(args),
        createModal:()=>{throw new Error('password must never be displayed')},closeModal:id=>closed.push(id),
        fetchWithTimeout:async(url,options)=>{posts.push({url,body:JSON.parse(options.body)});return {json:async()=>({result:'success'})}}};
    vm.createContext(context);vm.runInContext(source.slice(start,end),context);
    context.event={preventDefault(){},target:{reset(){reset=true}}};
    await vm.runInContext('handleChangeToken(event)',context);
    assert.deepEqual(saved,[['Lab12345',false]]);
    assert.equal(context.common_headers.authorization,'Bearer Lab12345');
    assert.deepEqual(posts,[{url:'/api/set_token',body:{token:'Lab12345'}}]);
    assert.deepEqual(closed,['#changeTokenModal']);assert.equal(reset,true);
    assert.equal(JSON.stringify(toasts).includes('Lab12345'),false);
});

test('background settings reload the selected job and keep runtime metadata out of saves', async () => {
    const source = fs.readFileSync(path.join(__dirname, 'static/app.js'), 'utf8');
    const start = source.indexOf('function showRuntimeSettings(');
    const end = source.indexOf("document.querySelector('#syncLocalTime')", start);
    const nodes = Object.fromEntries(['runtimeJob','runtimeConfig','runtimeState','saveRuntime','loadRuntime'].map(id=>['#'+id,{}]));
    nodes['#runtimeJob'].value = 'log_clear';
    const calls = [], errors = [], messages = [];
    let fail = false;
    const settings = {log_clear:{enabled:false,interval_hours:37},'time/config':{enabled:false,interval_hours:13}};
    const context = {document:{querySelector:id=>nodes[id]}, toast:message=>messages.push(message),
        report:fn=>async()=>{try{await fn()}catch(e){errors.push(e.message)}},
        request:async(url,options={})=>{
            calls.push({url,...options});if(fail)throw new Error('read failed');
            const job = url.slice(5);
            if(options.method==='POST')settings[job]=JSON.parse(JSON.stringify(options.json));
            return {data:{...settings[job],active:false,runtime:{state:'idle'}}};
        }};
    vm.createContext(context);vm.runInContext(source.slice(start,end),context);
    await nodes['#loadRuntime'].onclick();
    assert.equal(JSON.parse(nodes['#runtimeConfig'].value).interval_hours,37);
    assert.equal(nodes['#saveRuntime'].disabled,false);
    assert.equal(JSON.parse(nodes['#runtimeState'].textContent).active,false);
    nodes['#runtimeJob'].value='time/config';await nodes['#runtimeJob'].onchange();
    assert.deepEqual(JSON.parse(nodes['#runtimeConfig'].value),settings['time/config']);
    nodes['#runtimeConfig'].value='{"enabled":false,"interval_hours":17}';await nodes['#saveRuntime'].onclick();
    assert.deepEqual(JSON.parse(JSON.stringify(calls.at(-1).json)),{enabled:false,interval_hours:17});
    assert.match(messages.at(-1),/当前后台未启用/);
    nodes['#runtimeConfig'].value='stale input';await nodes['#loadRuntime'].onclick();
    assert.equal(JSON.parse(nodes['#runtimeConfig'].value).interval_hours,17);
    fail=true;nodes['#runtimeJob'].value='log_clear';await nodes['#runtimeJob'].onchange();
    assert.equal(nodes['#saveRuntime'].disabled,true);
    assert.deepEqual(errors,['read failed']);
});

function storage() {
    const values = new Map();
    return { getItem: k => values.get(k) ?? null, setItem: (k, v) => values.set(k, String(v)), removeItem: k => values.delete(k) };
}
function application(reply) {
    const calls = [];
    const context = { URL, URLSearchParams, Headers, Request, AbortController, setTimeout, clearTimeout,
        console: { warn() {}, error() {}, log() {} }, localStorage: storage(), sessionStorage: storage(), location: { origin: 'http://127.0.0.1:2433' } };
    context.fetch = async (url, options = {}) => {
        const call = { url: String(url), ...options, headers: new Headers(options.headers) };
        calls.push(call);
        const result = await reply(call);
        return { status: result.status ?? 200, ok: (result.status ?? 200) < 400, json: async () => result.body ?? {} };
    };
    context.window = context;
    vm.createContext(context);
    vm.runInContext(fs.readFileSync(path.join(__dirname, 'static/script/requests.js'), 'utf8'), context);
    return { context, calls, run: code => vm.runInContext(code, context) };
}

test('Wi-Fi selector reports dual-band state without mislabeling it as 5G only', async () => {
    const source = fs.readFileSync(path.join(__dirname, 'static/script/main.js'), 'utf8');
    const start = source.indexOf('    let initWIFISwitch = async () => {');
    const end = source.indexOf('\n    initWIFISwitch()', start);
    assert.ok(start >= 0 && end > start);
    for (const [on, bands, steering, expected] of [['1', [0, 1], '1', 'dual'], ['1', [0], '1', 'chip1'], ['1', [1], '0', 'chip2'], ['0', [0, 1], '1', 0]]) {
        const five = {disabled:false}, select = {style:{}, value:'', querySelector:()=>five}, info = {style:{}};
        const context = {URLSearchParams, document:{querySelector:id=>id === '#WIFI_SWITCH' ? select : info},
            initRequestData:async()=>true, initWIFIManagementForm:async()=>{},
            getData:async()=>({WiFiModuleSwitch:on, BandSteeringSwitch:steering, ResponseList:[0,1].map(i=>({ChipIndex:i, AccessPointSwitchStatus:bands.includes(i)?'1':'0'}))})};
        vm.createContext(context);
        await vm.runInContext(source.slice(start,end) + '\ninitWIFISwitch()', context);
        assert.equal(select.value, expected);
        assert.equal(five.disabled, steering !== '0');
    }
    const html = fs.readFileSync(path.join(__dirname, 'static/index.html'), 'utf8');
    assert.match(html, /<option value="dual" disabled /);
});

test('global Wi-Fi enable requests power only and leaves band policy untouched', async () => {
    const source = fs.readFileSync(path.join(__dirname, 'static/script/main.js'), 'utf8');
    const start = source.indexOf('    let changeWIFISwitch = async (e) => {');
    const end = source.indexOf('\n    let initSMBStatus', start);
    assert.ok(start >= 0 && end > start);
    for (const value of ['0','1']) {
        const calls = [];
        const context = {document:{querySelector:()=>({style:{}})}, initRequestData:async()=>true,
            login:async()=>true, postData:async(_,data)=>{calls.push(data);return {json:async()=>({result:'success'})}},
            createToast:()=>{},t:x=>x,setTimeout:f=>f(),initWIFISwitch:async()=>{}};
        vm.createContext(context);
        await vm.runInContext(source.slice(start,end) + '\nchangeWIFISwitch({target:{value:' + JSON.stringify(value) + '}})',context);
        assert.equal(calls.length,1);
        assert.deepEqual(JSON.parse(JSON.stringify(calls[0])),{goformId:'switchWiFiModule',SwitchOption:Number(value)});
    }
});

test('candidate login uses raw application token, without factory-password challenge', async () => {
    const app = application(c => ({ status: c.headers.get('authorization') === 'Bearer correct-token' ? 200 : 401 }));
    app.run("storeLoginToken('correct-token', false)");
    assert.equal(await app.run('login()'), true);
    assert.equal(app.calls[0].url, '/api/config');
    assert.equal(app.context.localStorage.getItem('ufi_remembered_token'), null);
    app.run("KANO_TOKEN = 'wrong-token'; common_headers.authorization = 'Bearer wrong-token'");
    assert.equal(await app.run('login()'), null);
    assert.equal(app.calls.length, 2);
    app.run('clearLoginToken()');
    assert.equal(app.context.sessionStorage.getItem('ufi_session_token'), null);
});

test('API auth is automatic, request content type survives, token is not sent to external services', async () => {
    const app = application(() => ({}));
    app.run("storeLoginToken('correct-token', true)");
    await app.run("postData(true, {goformId: 'WIFI_SWITCH', enabled: '1'})");
    assert.equal(app.calls[0].headers.get('content-type'), 'application/x-www-form-urlencoded');
    assert.equal(app.calls[0].headers.get('authorization'), 'Bearer correct-token');
    assert.match(app.calls[0].body.toString(), /goformId=WIFI_SWITCH/);
    await app.run("fetch('https://example.invalid/plugin', {headers: {authorization: 'Bearer correct-token'}})");
    assert.equal(app.calls[1].headers.has('authorization'), false);
    await app.run("fetch('https://example.invalid/plugin', {headers: {authorization: 'external-service-key'}})");
    assert.equal(app.calls[2].headers.get('authorization'), 'external-service-key');
});

test('one concurrent refresh pair is shared and supplemental null does not erase a real reading', async () => {
    const app = application(c => ({ body: c.url.includes('baseDeviceInfo') ? { cpu_usage: 25, battery: null } : { loginfo: 'ok', battery_value: '80' } }));
    const [first, second] = await Promise.all([app.run('getUFIData()'), app.run('getUFIData()')]);
    assert.equal(app.calls.length, 2);
    assert.equal(first.battery, '80');
    assert.equal(second.cpu_usage, 25);
    await app.run('getUFIData()');
    assert.equal(app.calls.length, 4);
});

test('unauthorized refresh expires the application session instead of rendering the error envelope', async () => {
    const app = application(() => ({ status: 401, body: { code: 401 } }));
    assert.equal((await app.run('getUFIData()')).__authInvalid, true);
});

test('service worker caches public assets and leaves API, private and external requests untouched', async () => {
    const listeners = {}, cached = [];
    const context = { URL, self: { location: { origin: 'http://127.0.0.1:2433' }, addEventListener: (name, fn) => { listeners[name] = fn; } },
        caches: { open: async () => ({ put: async request => cached.push(request.url) }) },
        fetch: async () => ({ ok: true, type: 'basic', clone: () => ({}) }) };
    vm.runInNewContext(fs.readFileSync(path.join(__dirname,'static/service-worker.js'),'utf8'), context);
    for (const url of ['/api/config','/api/terminal/','/api/uploads/test','https://example.invalid/script/main.js']) {
        let intercepted = false;
        listeners.fetch({request:{url:new URL(url,context.self.location.origin).href,method:'GET'},respondWith:()=>{intercepted=true;}});
        assert.equal(intercepted,false,url);
    }
    const pending = [];
    listeners.fetch({request:{url:context.self.location.origin+'/script/main.js',method:'GET'},respondWith:p=>pending.push(p),waitUntil:p=>pending.push(p)});
    await Promise.all(pending);
    assert.deepEqual(cached,[context.self.location.origin+'/script/main.js']);
});

// Plugin helpers must expose failures instead of turning an auth/error envelope
// into success=true with undefined output. user_shell has a structured result.
test('plugin shell helpers preserve HTTP failures and normalize command output', async () => {
    for (const [status, body, success, content] of [
        [401, {code:401,message:'authentication required'}, false, 'authentication required'],
        [403, {error:'shell compatibility is disabled'}, false, 'shell compatibility is disabled'],
        [200, {result:'root output'}, true, 'root output'],
        [200, {result:{done:true,content:'user output',exit_code:0}}, true, 'user output'],
        [200, {data:{output:'failed output',exit_code:7}}, false, 'failed output'],
        [200, {}, false, 'Invalid shell response'],
    ]) {
        const app = application(() => ({status,body}));
        const result = await app.run("runShellWithUser('id -u')");
        assert.equal(result.success, success);
        assert.equal(result.content, content);
        assert.equal(app.calls[0].headers.get('content-type'), 'application/json');
    }
});

test('homepage uses native signal bars instead of physical RSSI or a placeholder zero', async () => {
    const app=application(()=>({}));
    for(const [input,expected] of [
        [{network_signalbar:'5',rssi:0},5],
        [{network_signalbar:0,rssi:4},0],
        [{network_signalbar:null,rssi:-53},null],
        [{network_signalbar:'',rssi:''},null],
        [{rssi:'3'},3],
    ]) assert.equal(app.run(`getNetworkSignalBars(${JSON.stringify(input)})`),expected);
    const source=fs.readFileSync(path.join(__dirname,'static/script/main.js'),'utf8');
    assert.match(source,/const signalBars = getNetworkSignalBars\(res\)/);
    assert.match(source,/kano_getSignalEmoji\(signalBars\)/);
});

test('homepage requests every measured NR field it renders', async () => {
    const app=application(()=>({}));await app.run('getUFIData()');
    const cmd=new URL(app.calls[0].url,'http://127.0.0.1:2433').searchParams.get('cmd').split(',');
    for(const key of ['Z5g_rsrp','Nr_snr','nr_rsrq','nr_rssi','Nr_bands','Nr_fcn','Nr_bands_widths','Nr_pci','Nr_cell_id'])assert(cmd.includes(key),key);
});

test('current cell preserves PCI zero and does not invent a cell or strength on missing data', () => {
    const source=fs.readFileSync(path.join(__dirname,'static/script/main.js'),'utf8');
    const start=source.indexOf("                const current_cell = document.querySelector('#CURRENT_CELL')");
    const end=source.indexOf('\n            } catch (e)',start);assert(start>=0&&end>start);
    for(const present of [true,false]){
        const cell={innerHTML:''},button={},selected=[],bars=[];
        const res=present?{Nr_pci:0,Nr_fcn:504990,Nr_bands:'41',Nr_snr:0}:{};
        vm.runInNewContext(source.slice(start,end),{res,document:{querySelector:id=>id==='#CURRENT_CELL'?cell:button},
            notNullOrundefinedOrIsShow:(obj,key)=>obj[key]!==null&&obj[key]!==undefined&&obj[key]!=='',
            t:key=>key,createToast:()=>{},onSelectCellRow:(...args)=>selected.push(args),
            kano_parseSignalBar:value=>{bars.push(value);return `meter:${value}`;}});
        button.onclick();
        if(present){assert.match(cell.innerHTML,/<td>0<\/td>/);assert.match(cell.innerHTML,/<td>—<\/td>/);assert.deepEqual(selected,[[0,504990]]);assert.deepEqual(bars,[0]);}
        else{assert.match(cell.innerHTML,/no_cell_connected/);assert.deepEqual(selected,[]);assert.deepEqual(bars,[]);}
    }
});
