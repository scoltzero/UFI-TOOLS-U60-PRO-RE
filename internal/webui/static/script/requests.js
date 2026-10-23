function SHA256(e) { function t(e, t) { var n = (65535 & e) + (65535 & t); return (e >> 16) + (t >> 16) + (n >> 16) << 16 | 65535 & n } function n(e, t) { return e >>> t | e << 32 - t } function r(e, t) { return e >>> t } function o(e, t, n) { return e & t ^ ~e & n } function i(e, t, n) { return e & t ^ e & n ^ t & n } function a(e) { return n(e, 2) ^ n(e, 13) ^ n(e, 22) } function s(e) { return n(e, 6) ^ n(e, 11) ^ n(e, 25) } function c(e) { return n(e, 7) ^ n(e, 18) ^ r(e, 3) } function u(e) { return n(e, 17) ^ n(e, 19) ^ r(e, 10) } var l = 8, d = 1; return e = function (e) { e = e.replace(/\\r\\n/g, "\\n"); for (var t = "", n = 0; n < e.length; n++) { var r = e.charCodeAt(n); r < 128 ? t += String.fromCharCode(r) : r > 127 && r < 2048 ? (t += String.fromCharCode(r >> 6 | 192), t += String.fromCharCode(63 & r | 128)) : (t += String.fromCharCode(r >> 12 | 224), t += String.fromCharCode(r >> 6 & 63 | 128), t += String.fromCharCode(63 & r | 128)) } return t }(e), function (e) { for (var t = d ? "0123456789ABCDEF" : "0123456789abcdef", n = "", r = 0; r < 4 * e.length; r++)n += t.charAt(e[r >> 2] >> 8 * (3 - r % 4) + 4 & 15) + t.charAt(e[r >> 2] >> 8 * (3 - r % 4) & 15); return n }(function (e, n) { var r, l, d, p, h, f, m, g, _, b, v, $, S = new Array(1116352408, 1899447441, 3049323471, 3921009573, 961987163, 1508970993, 2453635748, 2870763221, 3624381080, 310598401, 607225278, 1426881987, 1925078388, 2162078206, 2614888103, 3248222580, 3835390401, 4022224774, 264347078, 604807628, 770255983, 1249150122, 1555081692, 1996064986, 2554220882, 2821834349, 2952996808, 3210313671, 3336571891, 3584528711, 113926993, 338241895, 666307205, 773529912, 1294757372, 1396182291, 1695183700, 1986661051, 2177026350, 2456956037, 2730485921, 2820302411, 3259730800, 3345764771, 3516065817, 3600352804, 4094571909, 275423344, 430227734, 506948616, 659060556, 883997877, 958139571, 1322822218, 1537002063, 1747873779, 1955562222, 2024104815, 2227730452, 2361852424, 2428436474, 2756734187, 3204031479, 3329325298), y = new Array(1779033703, 3144134277, 1013904242, 2773480762, 1359893119, 2600822924, 528734635, 1541459225), C = new Array(64); e[n >> 5] |= 128 << 24 - n % 32, e[15 + (n + 64 >> 9 << 4)] = n; for (var _ = 0; _ < e.length; _ += 16) { r = y[0], l = y[1], d = y[2], p = y[3], h = y[4], f = y[5], m = y[6], g = y[7]; for (var b = 0; b < 64; b++)C[b] = b < 16 ? e[b + _] : t(t(t(u(C[b - 2]), C[b - 7]), c(C[b - 15])), C[b - 16]), v = t(t(t(t(g, s(h)), o(h, f, m)), S[b]), C[b]), $ = t(a(r), i(r, l, d)), g = m, m = f, f = h, h = t(p, v), p = d, d = l, l = r, r = t(v, $); y[0] = t(r, y[0]), y[1] = t(l, y[1]), y[2] = t(d, y[2]), y[3] = t(p, y[3]), y[4] = t(h, y[4]), y[5] = t(f, y[5]), y[6] = t(m, y[6]), y[7] = t(g, y[7]) } return y }(function (e) { for (var t = Array(), n = (1 << l) - 1, r = 0; r < e.length * l; r += l)t[r >> 5] |= (e.charCodeAt(r / l) & n) << 24 - r % 32; return t }(e), e.length * l)) }
function gsmEncode(text) { function encodeText(text) { let encoded = []; for (let i = 0; i < text.length; i++) { const char = text[i]; const codePoint = char.codePointAt(0); if (codePoint <= 0xFFFF) { encoded.push((codePoint >> 8) & 0xFF); encoded.push(codePoint & 0xFF) } else { const highSurrogate = 0xD800 + ((codePoint - 0x10000) >> 10); const lowSurrogate = 0xDC00 + ((codePoint - 0x10000) & 0x3FF); encoded.push((highSurrogate >> 8) & 0xFF); encoded.push(highSurrogate & 0xFF); encoded.push((lowSurrogate >> 8) & 0xFF); encoded.push(lowSurrogate & 0xFF) } } return encoded } function toHexString(byteArray) { return byteArray.map(byte => byte.toString(16).padStart(2, '0')).join('') } const encodedBytes = encodeText(text); return toHexString(encodedBytes) }
//注意，如果是在f50本机内发起请求，请将请求端口更改为8080
let KANO_baseURL = '/api'
let KANO_PASSWORD = null
let KANO_TOKEN = storedLoginToken()
let ACCEPT_TERMS = false
let KANO_COOKIE = null

let loginMethod = localStorage.getItem('login_method') == "1" ? "1" : "0"; //1新方法，0旧方法

const originFetch = window.fetch;

// Authentication belongs to this candidate service. Never attach its token to
// third-party plugin URLs.
function storedLoginToken() {
    return sessionStorage.getItem('ufi_session_token') || localStorage.getItem('ufi_remembered_token') || ''
}
function storeLoginToken(token, remember) {
    clearLoginToken()
    if (remember) localStorage.setItem('ufi_remembered_token', token)
    else sessionStorage.setItem('ufi_session_token', token)
}
function clearLoginToken() {
    sessionStorage.removeItem('ufi_session_token')
    localStorage.removeItem('ufi_remembered_token')
    localStorage.removeItem('kano_sms_token')
    localStorage.removeItem('kano_sms_pwd')
    localStorage.removeItem('kano_remembered_loginfo')
}
(() => {
    const original = window.fetch
    window.fetch = (input, init = {}) => {
        const url = new URL(typeof input === 'string' ? input : input.url, window.location.origin)
        const headers = new Headers(init.headers || (typeof Request !== 'undefined' && input instanceof Request ? input.headers : undefined))
        if (url.origin === window.location.origin && url.pathname.startsWith('/api/')) {
            const token = KANO_TOKEN || storedLoginToken()
            if (token && (!headers.get('authorization') || headers.get('authorization') === 'null')) headers.set('authorization', 'Bearer ' + token)
        }
        const ownToken = KANO_TOKEN || storedLoginToken()
        if (url.origin !== window.location.origin && ownToken && [ownToken, 'Bearer ' + ownToken].includes(headers.get('authorization'))) headers.delete('authorization')
        return original(input, { ...init, headers })
    }
})();

//登录
const common_headers = { authorization: KANO_TOKEN ? 'Bearer ' + KANO_TOKEN : '' }

// Validate the candidate token without a second ZTE password/challenge login.
let login = async () => {
    try {
        const res = await fetchWithTimeout(KANO_baseURL + '/config', { headers: { ...common_headers } }, 5000)
        return res.ok ? true : null
    } catch { return null }
}
const logout = async () => { KANO_COOKIE = null; return true }
const postData = async (_session, data = {}) => {
    return fetchWithTimeout(KANO_baseURL + '/goform/goform_set_cmd_process', {
        method: 'POST',
        headers: { ...common_headers, 'Content-Type': 'application/x-www-form-urlencoded' },
        body: new URLSearchParams({ ...data, isTest: 'false' })
    })
}

const getData = async (data = new URLSearchParams({})) => {
    data.append('isTest', 'false')
    data.append('_', Date.now())
    const res = await fetchWithTimeout(KANO_baseURL + "/goform/goform_get_cmd_process?" + data.toString(), {
        method: "GET",
        headers: {
            ...common_headers,
            "Content-Type": "application/x-www-form-urlencoded",
        },
    }, 5000)
    return await res.json()
}

const reboot = async (cookie) => {
    const res = await postData(cookie, {
        goformId: 'REBOOT_DEVICE',
    })
    return res
}

// 发送短信
const sendSms_UFI = async ({ content, number }) => {
    if (!content) throw new Error('请提供短信内容')
    if (!number) throw new Error('请提供手机号')
    const cookie = await login()
    const res = await postData(cookie, {
        goformId: 'SEND_SMS',
        Number: number,
        MessageText: content
    })
    await logout(cookie)
    return await res.json()
}

//删除短信
const removeSmsById = async (id) => {
    if (!id) throw new Error('请提供短信id')
    const cookie = await login()
    const res = await postData(cookie, {
        goformId: 'DELETE_SMS',
        msg_id: id,
        notCallback: true,
    })
    await logout(cookie)
    return await res.json()
}

// 已读短信
const readSmsByIds = async (ids) => {
    if (!ids || !Array.isArray(ids) || ids.length === 0) {
        throw new Error('请提供短信id数组');
    }

    const cookie = await login();
    const results = [];

    for (const id of ids) {
        try {
            const response = await postData(cookie, {
                goformId: 'SET_MSG_READ',
                msg_id: id,
                notCallback: true,
            });

            const data = await response.json();
            results.push({ id, success: data?.result == 'success', data });
        } catch (err) {
            console.error(`处理短信 ID ${id} 时出错:`, err);
            results.push({ id, success: false, error: err.message || String(err) });
        }
    }

    await logout(cookie);
    return results;
};

//获取短信列表（base64编码）
const getSmsInfo = async (page = 0, pageSize = 500) => {
    const params = new URLSearchParams()
    params.append('_', Date.now().toString())
    const res = await fetch(KANO_baseURL + `/goform/goform_get_cmd_process?multi_data=1&isTest=false&cmd=sms_data_total&page=${page}&data_per_page=${pageSize}&mem_store=1&tags=100&order_by=order by id desc&` + params, {
        headers: {
            ...common_headers
        }
    })
    return await res.json()
}

// Signal icons consume a 0..5 bar count, not physical RSSI in dBm.
const getNetworkSignalBars = (data) => {
    for (const value of [data.network_signalbar, data.rssi]) {
        if ((typeof value !== 'number' && typeof value !== 'string') || String(value).trim() === '') continue
        const bars = Number(value)
        if (Number.isInteger(bars) && bars >= 0 && bars <= 5) return bars
    }
    return null
}

let ufiDataInFlight = null
const getUFIData = () => {
    if (ufiDataInFlight) return ufiDataInFlight
    ufiDataInFlight = (async () => {
        const controller = new AbortController()
        const timer = setTimeout(() => controller.abort(), 5000)
        try {
            const cmd = 'usb_port_switch,battery_charging,sms_received_flag,sms_unread_num,sms_sim_unread_num,sim_msisdn,data_volume_limit_switch,battery_value,battery_vol_percent,network_signalbar,network_rssi,cr_version,iccid,imei,imsi,ipv6_wan_ipaddr,lan_ipaddr,mac_address,msisdn,network_information,Lte_ca_status,rssi,Z5g_rsrp,Nr_snr,nr_rsrq,nr_rssi,Nr_bands,Nr_fcn,Nr_bands_widths,Nr_pci,Nr_cell_id,lte_rsrp,Lte_snr,lte_rsrq,lte_rssi,Lte_bands,wifi_access_sta_num,loginfo,data_volume_alert_percent,data_volume_limit_size,realtime_rx_thrpt,realtime_tx_thrpt,realtime_time,monthly_tx_bytes,monthly_rx_bytes,monthly_time,network_type,network_provider,ppp_status'
            const [state, metrics] = await Promise.all([
                fetch(`${KANO_baseURL}/goform/goform_get_cmd_process?cmd=${cmd}&_=${Date.now()}`, { signal: controller.signal }),
                fetch(`${KANO_baseURL}/baseDeviceInfo`, { signal: controller.signal })
            ])
            if ([state.status, metrics.status].some(s => s === 401 || s === 403)) return { __authInvalid: true }
            if (!state.ok) throw new Error('Device status unavailable')
            const data = await state.json()
            const details = metrics.ok ? await metrics.json() : {}
            // An unavailable supplemental sensor must not erase a valid source.
            const available = Object.fromEntries(Object.entries(details).filter(([, value]) => value !== null && value !== undefined))
            return { ...data, ...available, battery: available.battery ?? data.battery_value ?? data.battery_vol_percent ?? null }
        } catch (error) {
            console.warn('Device refresh failed:', error.message)
            return null
        } finally { clearTimeout(timer) }
    })().finally(() => { ufiDataInFlight = null })
    return ufiDataInFlight
}

function originFetchWithTimeout(url = '', options = {}, timeout = 10000) {
    const controller = new AbortController()
    const tid = setTimeout(() => controller.abort(), timeout);
    return fetch(url, {
        ...options,
        signal: controller.signal,
    })
        .then(response => {
            // 处理响应
            return response
        })
        .catch(err => {
            if (err.name === 'AbortError') {
                console.error('请求超时')
            } else {
                console.error('请求失败', err)
            }
            throw err
        }).finally(() => {
            clearTimeout(tid)
        })
}




function fetchWithTimeout(url = '', options = {}, timeout = 10000) {
    const controller = new AbortController()
    const tid = setTimeout(() => controller.abort(), timeout);
    return fetch(url, {
        ...options,
        signal: controller.signal,
        headers: (() => {
            const headers = new Headers(common_headers)
            new Headers(options.headers || {}).forEach((value, key) => headers.set(key, value))
            return headers
        })()
    })
        .then(response => {
            // 处理响应
            return response
        })
        .catch(err => {
            if (err.name === 'AbortError') {
                console.error('请求超时')
            } else {
                console.error('请求失败', err)
            }
            throw err
        }).finally(() => {
            clearTimeout(tid)
        })
}

//查流量使用情况
async function getDataUsage() {
    try {
        const res = await getData(new URLSearchParams({
            cmd: 'flux_data_volume_limit_switch,data_volume_limit_switch,data_volume_limit_unit,data_volume_limit_size,data_volume_alert_percent,monthly_tx_bytes,monthly_rx_bytes,monthly_time,wan_auto_clear_flow_data_switch,traffic_clear_date,',
            multi_data: 1
        }))
        return res
    } catch {
        return null
    }
}

//adb保活
async function adbKeepAlive() {
    try {
        const { result } = await (await fetch(`${KANO_baseURL}/adb_alive`, {
            headers: common_headers
        })).json()
        if (result == undefined || result == null) return false
        return result == "true" ? true : false
    } catch {
        return false
    }

}

//自定义头部
const getCustomHead = async () => {
    try {
        const { text } = await (await fetchWithTimeout(`${KANO_baseURL}/get_custom_head`, {
            headers: { ...common_headers }
        })).json()
        return text || ''
    } catch (e) {
        return '';
    }
}
const setCustomHead = async (text = "") => {
    try {
        const { result, error } = await (await fetchWithTimeout(`${KANO_baseURL}/set_custom_head`, {
            headers: { ...common_headers },
            method: "POST",
            body: JSON.stringify({
                text: text
            })
        })).json()
        return {
            result, error
        }
    } catch (e) {
        return false
    }
}

// Keep legacy plugin helpers consistent for HTTP/auth failures and both shell
// response forms. An error envelope must never become a successful empty result.
const runShellCommand = async (endpoint, cmd, timeout) => {
    try {
        const res = await fetchWithTimeout(`${KANO_baseURL}/${endpoint}`, {
            method: 'POST',
            headers: { ...common_headers, 'Content-Type': 'application/json' },
            body: JSON.stringify({ command: cmd.trim(), timeout })
        }, timeout)
        const data = await res.json()
        if (!res.ok || data.error || (data.code !== undefined && data.code !== 0)) {
            return { success: false, content: data.error || data.message || `HTTP ${res.status}` }
        }
        const result = data.result ?? data.data
        if (typeof result === 'string') return { success: true, content: result }
        if (result && typeof (result.content ?? result.output) === 'string') {
            return { success: !result.timed_out && (result.exit_code === undefined || result.exit_code === 0), content: result.content ?? result.output }
        }
        return { success: false, content: 'Invalid shell response' }
    } catch (e) {
        return { success: false, content: e.message }
    }
}
const runShellWithRoot = (cmd = '', timeout = 10000) => runShellCommand('root_shell', cmd, timeout)
const runShellWithUser = (cmd = '', timeout = 10000) => runShellCommand('user_shell', cmd, timeout)

const updateAdminPsw = async (newPsw) => {
    try {
        const res = await fetchWithTimeout(`${KANO_baseURL}/update_admin_pwd`, {
            method: "POST",
            headers: common_headers,
            body: JSON.stringify({
                password: newPsw
            })
        }, 5000)
        const { result, error } = await res.json()
        return { result, error }
    } catch (e) {
        return { result: null, error: e.message }
    }
}

// apn
const getAPNData = async () => {
    try {
        const res = await getData(new URLSearchParams({
            cmd: 'apn_interface_version,APN_config0,APN_config1,APN_config2,APN_config3,APN_config4,APN_config5,APN_config6,APN_config7,APN_config8,APN_config9,APN_config10,APN_config11,APN_config12,APN_config13,APN_config14,APN_config15,APN_config16,APN_config17,APN_config18,APN_config19,ipv6_APN_config0,ipv6_APN_config1,ipv6_APN_config2,ipv6_APN_config3,ipv6_APN_config4,ipv6_APN_config5,ipv6_APN_config6,ipv6_APN_config7,ipv6_APN_config8,ipv6_APN_config9,ipv6_APN_config10,ipv6_APN_config11,ipv6_APN_config12,ipv6_APN_config13,ipv6_APN_config14,ipv6_APN_config15,ipv6_APN_config16,ipv6_APN_config17,ipv6_APN_config18,ipv6_APN_config19,apn_m_profile_name,profile_name,apn_wan_dial,apn_select,apn_pdp_type,apn_pdp_select,apn_pdp_addr,index,apn_Current_index,apn_auto_config,apn_ipv6_apn_auto_config,apn_mode,apn_wan_apn,apn_ppp_auth_mode,apn_ppp_username,apn_ppp_passwd,dns_mode,prefer_dns_manual,standby_dns_manual,apn_ipv6_wan_apn,apn_ipv6_pdp_type,apn_ipv6_ppp_auth_mode,apn_ipv6_ppp_username,apn_ipv6_ppp_passwd,ipv6_dns_mode,ipv6_prefer_dns_manual,ipv6_standby_dns_manual,apn_num_preset,wan_apn_ui,profile_name_ui,pdp_type_ui,ppp_auth_mode_ui,ppp_username_ui,ppp_passwd_ui,dns_mode_ui,prefer_dns_manual_ui,standby_dns_manual_ui,ipv6_wan_apn_ui,ipv6_ppp_auth_mode_ui,ipv6_ppp_username_ui,ipv6_ppp_passwd_ui,ipv6_dns_mode_ui,ipv6_prefer_dns_manual_ui,ipv6_standby_dns_manual_ui',
            multi_data: 1
        }))
        return res
    } catch {
        return null
    }
}

//deleteAPNProfile
const deleteAPNProfile = async (index) => {
    if (index == undefined || index == null) throw new Error('请提供index')
    const res = await postData(await login(), {
        goformId: "APN_PROC_EX",
        index,
        apn_mode: "manual",
        apn_action: "delete"
    })
    return res.json()
}

//saveAPNProfile
const saveAPNProfile = async (data) => {
    const res = await postData(await login(), {
        goformId: "APN_PROC_EX",
        apn_mode: "manual",
        apn_action: "save",
        ...data
    })
    return res.json()
}

//switchAPNAuto
const switchAPNAuto = async ({ isAuto = true, index = 0 }) => {
    const formData = {
        goformId: "APN_PROC_EX",
        apn_mode: isAuto ? "auto" : "manual",

    }
    const manualData = {
        apn_action: "set_default",
        set_default_flag: '1',
        apn_pdp_type: '',
        index
    }
    const data = isAuto ? formData : { ...formData, ...manualData }
    const res = await postData(await login(), data)
    return res.json()
}

// check Terms acceptance
const getTermsAcceptance = async () => {
    const res = await (await fetchWithTimeout(`${KANO_baseURL}/version_info`)).json()
    ACCEPT_TERMS = res.accept_terms && res.accept_terms.toString() == 'true'
    if (ACCEPT_TERMS) {
        return true
    }
    return false
}

// check sim pin
const getSimPinStatus = async () => {
    try {
        const res = await getData(new URLSearchParams({
            cmd: "modem_main_state,mc_modem_main_state,puknumber,pinnumber,sim_pinnumber"
        }))
        return {
            modem_main_state: res.modem_main_state || res.mc_modem_main_state,
            puknumber: res.puknumber,
            pinnumber: res.pinnumber || res.sim_pinnumber
        }
    }
    catch (e) {
        console.error("getSimPinStatus Error:", e)
        return null
    }
}

const getNetConnInfo = async () => {
    try {
        const res = await (await fetchWithTimeout(`${KANO_baseURL}/connInfo`)).json()
        if (res.result == 'success') {
            return res.data
        }
    } catch (e) {
        console.error("getNetConnInfo Error:", e)
    }
    return null
}

//seConntHostName
const seConntHostName = async (mac, hostname) => {
    const formData = {
        goformId: "EDIT_HOSTNAME",
        mac,
        hostname
    }
    const res = await postData(await login(), formData)
    return res.json()
}

const getDailyUsageRange = async (start, endTime) => {
    start = start instanceof Date ? start.toISOString().slice(0,10) : start
    endTime = endTime instanceof Date ? endTime.toISOString().slice(0,10) : endTime
    const res = await fetchWithTimeout(`${KANO_baseURL}/cellularUsage?start=${encodeURIComponent(start)}&end=${encodeURIComponent(endTime)}`)
    const data = await res.json()
    if (!res.ok || !Array.isArray(data.usage)) throw new Error(data.message || '无法读取 UTC 流量统计')
    return data.usage
};
