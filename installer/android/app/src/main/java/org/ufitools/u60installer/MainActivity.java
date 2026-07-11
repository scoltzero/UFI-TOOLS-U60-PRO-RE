// SPDX-License-Identifier: AGPL-3.0-or-later
package org.ufitools.u60installer;

import android.app.Activity;
import android.content.Intent;
import android.net.Uri;
import android.os.Bundle;
import android.provider.OpenableColumns;
import android.text.InputType;
import android.view.View;
import android.widget.*;
import java.io.*;
import java.net.*;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.text.SimpleDateFormat;
import java.util.*;
import java.util.concurrent.Executors;
import org.json.JSONArray;
import org.json.JSONObject;

public final class MainActivity extends Activity {
    private static final int PICK = 7, PICK_ORIGINAL = 8;
    private final java.util.concurrent.ExecutorService executor = Executors.newSingleThreadExecutor();
    private EditText base, routerPassword, session, confirmation;
    private CheckBox execute;
    private TextView selected, output;
    private Uri container, rollbackContainer;

    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        LinearLayout root = new LinearLayout(this);
        root.setOrientation(LinearLayout.VERTICAL); root.setPadding(32, 32, 32, 32);
        TextView title = text("U60 Pro 开源安装器", 24); root.addView(title);
        root.addView(text("默认只预览。执行必须选择能恢复当前服务与 rc.local 的回滚容器。", 15));
        base = field("http://192.168.98.1"); add(root, "设备管理地址", base);
        routerPassword = field(""); routerPassword.setInputType(InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_VARIATION_PASSWORD); add(root, "路由器 Web 密码（推荐；不保存）", routerPassword);
        session = field(""); add(root, "已有 UBUS 会话（与密码二选一）", session);
        Button choose = new Button(this); choose.setText(R.string.choose_container); choose.setOnClickListener(v -> pick()); root.addView(choose);
        selected = text("尚未选择文件", 14); root.addView(selected);
        Button chooseOriginal = new Button(this); chooseOriginal.setText(R.string.choose_rollback); chooseOriginal.setOnClickListener(v -> pick(PICK_ORIGINAL)); root.addView(chooseOriginal);
        execute = new CheckBox(this); execute.setText("执行设备恢复（不勾选则只预览）"); root.addView(execute);
        confirmation = field(""); add(root, "执行确认：输入 RESTORE U60 PRO", confirmation);
        Button start = new Button(this); start.setText("开始"); start.setOnClickListener(v -> start()); root.addView(start);
        output = text("等待操作", 13); output.setTextIsSelectable(true);
        ScrollView scroll = new ScrollView(this); scroll.addView(output); root.addView(scroll, new LinearLayout.LayoutParams(-1, 0, 1));
        setContentView(root);
    }

    private void pick() { pick(PICK); }
    private void pick(int request) { startActivityForResult(new Intent(Intent.ACTION_OPEN_DOCUMENT).setType("application/octet-stream").addCategory(Intent.CATEGORY_OPENABLE), request); }
    @Override protected void onActivityResult(int request, int result, Intent data) {
        super.onActivityResult(request, result, data);
        if (request == PICK && result == RESULT_OK && data != null) { container = data.getData(); selected.setText(name(container)); }
        if (request == PICK_ORIGINAL && result == RESULT_OK && data != null) { rollbackContainer = data.getData(); toast("已选择回滚容器"); }
    }
    private void start() {
        if (container == null) { toast("请先选择恢复容器"); return; }
        boolean mutate = execute.isChecked();
        if (mutate && !"RESTORE U60 PRO".equals(confirmation.getText().toString())) { toast("执行确认不匹配"); return; }
        if (mutate && rollbackContainer == null) { toast("执行前必须选择已验证的回滚容器"); return; }
        String targetBase = base.getText().toString().trim();
        String targetSession = session.getText().toString().trim();
        String targetPassword = routerPassword.getText().toString();
        if (!targetSession.isEmpty() && !targetPassword.isEmpty()) { toast("UBUS 会话和路由器密码只能填写一项"); return; }
        if (mutate && targetSession.isEmpty() && targetPassword.isEmpty()) { toast("执行需要路由器密码或已授权 UBUS 会话"); return; }
        output.setText(mutate ? "处理中，请勿断电…" : "正在验证预览…");
        executor.execute(() -> {
            try {
                byte[] candidate = read(container, 128 << 20);
                Installer installer = new Installer(targetBase, targetSession);
                if (!mutate) { show("预览通过\n容器大小: " + candidate.length + " 字节\n目标: " + installer.origin); return; }
                if (!targetPassword.isEmpty()) installer.login(targetPassword);
                File backup = new File(getFilesDir(), "rollback-" + new SimpleDateFormat("yyyyMMdd-HHmmss", Locale.US).format(new Date()) + ".bin");
                byte[] rollback = read(rollbackContainer, 128 << 20); write(backup, rollback);
                installer.upload(candidate); installer.restore();
                if (installer.waitRestart()) { show("安装成功\n回滚容器副本: " + backup.getAbsolutePath()); return; }
                installer.retryUpload(rollback); installer.retryRestore();
                if (!installer.waitRestart()) throw new IOException("回滚已触发，但服务未恢复健康");
                show("候选服务未恢复健康；回滚后服务已恢复\n备份: " + backup.getAbsolutePath());
            } catch (Exception e) { show("失败: " + e.getMessage()); }
        });
    }

    private final class Installer {
        final String origin;
        String sid;
        Installer(String raw, String session) throws Exception {
            URL url = new URL(raw);
            if (!("http".equals(url.getProtocol()) || "https".equals(url.getProtocol())) || url.getUserInfo() != null || !privateHost(url.getHost())) throw new IOException("仅允许私有/回环设备地址");
            origin = url.getProtocol() + "://" + url.getAuthority();
            sid = session.isEmpty() ? "00000000000000000000000000000000" : session;
            if (!sid.matches("[0-9a-fA-F]{32}")) throw new IOException("UBUS 会话必须是 32 位十六进制");
        }
        void login(String password) throws Exception {
            JSONObject info = call("00000000000000000000000000000000", "zwrt_web", "web_login_info", new JSONObject());
            String salt = info.optString("zte_web_sault", "");
            if (salt.isEmpty()) throw new IOException("路由器未返回登录盐");
            JSONObject args = new JSONObject(); args.put("password", sha256Upper(sha256Upper(password) + salt));
            JSONObject result = call("00000000000000000000000000000000", "zwrt_web", "web_login", args);
            String loggedIn = result.optString("result", "");
            String newSession = result.optString("ubus_rpc_session", "");
            if (!"0".equals(loggedIn) || !newSession.matches("[0-9a-fA-F]{32}")) throw new IOException("路由器密码错误或登录会话无效");
            sid = newSession;
        }
        void restore() throws Exception { rpc("device_restore_proc"); }
        void rpc(String procedure) throws Exception {
            JSONObject args = new JSONObject(); args.put("procType", "zwrt_web");
            JSONObject result = call(sid, "zwrt_mc.device.manager", procedure, args);
            if (!"success".equalsIgnoreCase(result.optString("result", ""))) throw new IOException("UBUS 操作失败");
        }
        JSONObject call(String callSession, String object, String procedure, JSONObject args) throws Exception {
            JSONArray params = new JSONArray(); params.put(callSession); params.put(object); params.put(procedure); params.put(args);
            JSONObject call = new JSONObject(); call.put("jsonrpc", "2.0"); call.put("id", 1); call.put("method", "call"); call.put("params", params);
            JSONArray payload = new JSONArray(); payload.put(call);
            String response = new String(request("POST", "/ubus/?t=" + System.currentTimeMillis(), payload.toString().getBytes(StandardCharsets.UTF_8), "application/json", 1 << 20), StandardCharsets.UTF_8);
            JSONArray decoded = new JSONArray(response);
            if (decoded.length() != 1 || decoded.getJSONObject(0).has("error")) throw new IOException("UBUS 返回错误");
            JSONArray result = decoded.getJSONObject(0).getJSONArray("result");
            if (result.length() < 2 || result.getInt(0) != 0) throw new IOException("UBUS 调用失败");
            return result.getJSONObject(1);
        }
        void upload(byte[] data) throws Exception {
            String boundary = "----UFI" + UUID.randomUUID().toString().replace("-", "");
            ByteArrayOutputStream body = new ByteArrayOutputStream();
            body.write(("--"+boundary+"\r\nContent-Disposition: form-data; name=\"filename\"\r\n\r\n/tmp/back_parameter\r\n--"+boundary+"\r\nContent-Disposition: form-data; name=\"filedata\"; filename=\"backup_restore.bin\"\r\nContent-Type: application/octet-stream\r\n\r\n").getBytes(StandardCharsets.UTF_8));
            body.write(data); body.write(("\r\n--"+boundary+"--\r\n").getBytes(StandardCharsets.UTF_8));
            String response = new String(request("POST", "/cgi-bin/cgi-upload", body.toByteArray(), "multipart/form-data; boundary="+boundary, 1 << 20), StandardCharsets.UTF_8).toLowerCase(Locale.ROOT);
            if (!(response.contains("checksum") && response.contains("sha256sum"))) throw new IOException("上传未返回双校验确认");
        }
        boolean waitRestart() throws InterruptedException { boolean down=false; long end=System.currentTimeMillis()+300000; while(System.currentTimeMillis()<end){ boolean ok=health(); if(!ok)down=true; else if(down)return true; Thread.sleep(3000); } return false; }
        boolean health() { try { request("GET", ":2333/healthz", null, null, 4096); return true; } catch(Exception e){ return false; } }
        void retryUpload(byte[] data) throws Exception { retry(() -> upload(data)); }
        void retryRestore() throws Exception { retry(this::restore); }
        void retry(Action action) throws Exception { Exception last=null; for(int i=0;i<20;i++){try{action.run();return;}catch(Exception e){last=e;Thread.sleep(3000);}}throw last; }
        byte[] request(String method, String path, byte[] body, String contentType, int max) throws Exception {
            URL target = path.startsWith(":") ? new URL(origin.replaceFirst("(:\\d+)?$", path.substring(0, path.indexOf('/'))) + path.substring(path.indexOf('/'))) : new URL(origin + path);
            HttpURLConnection c=(HttpURLConnection)target.openConnection(); c.setConnectTimeout(5000); c.setReadTimeout(45000); c.setInstanceFollowRedirects(false); c.setRequestMethod(method);
            if(path.startsWith("/ubus/")){c.setRequestProperty("Z-Mode","0");c.setRequestProperty("Z-Tag","");c.setRequestProperty("Origin",origin);c.setRequestProperty("Referer",origin+"/");c.setRequestProperty("X-Requested-With","XMLHttpRequest");if(!"00000000000000000000000000000000".equals(sid))c.setRequestProperty("Cookie","webtoken="+sid);}
            if(path.equals("/cgi-bin/cgi-upload")){c.setRequestProperty("Origin",origin);c.setRequestProperty("Referer",origin+"/");c.setRequestProperty("X-Requested-With","XMLHttpRequest");c.setRequestProperty("Accept","text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8");c.setRequestProperty("Accept-Language","zh-CN,zh;q=0.9,en;q=0.8");c.setRequestProperty("Cache-Control","no-cache");c.setRequestProperty("Pragma","no-cache");c.setRequestProperty("Upgrade-Insecure-Requests","1");if(!"00000000000000000000000000000000".equals(sid))c.setRequestProperty("Cookie","webtoken="+sid);}
            if(body!=null){c.setDoOutput(true);c.setRequestProperty("Content-Type",contentType);try(OutputStream out=c.getOutputStream()){out.write(body);}}
            int status=c.getResponseCode(); if(status<200||status>=300)throw new IOException("HTTP "+status);
            try(InputStream in=c.getInputStream()){return read(in,max);} finally {c.disconnect();}
        }
    }

    private interface Action { void run() throws Exception; }
    private static String sha256Upper(String value) throws Exception { byte[] digest=MessageDigest.getInstance("SHA-256").digest(value.getBytes(StandardCharsets.UTF_8));StringBuilder out=new StringBuilder(64);for(byte b:digest)out.append(String.format(Locale.ROOT,"%02X",b&255));return out.toString(); }
    private static boolean privateHost(String host) { try { if (!("localhost".equalsIgnoreCase(host) || host.matches("[0-9.]+") || host.contains(":"))) return false; InetAddress a=InetAddress.getByName(host); return a.isAnyLocalAddress()||a.isLoopbackAddress()||a.isSiteLocalAddress()||a.isLinkLocalAddress(); } catch(Exception e){return false;} }
    private byte[] read(Uri uri,int max)throws Exception{try(InputStream in=getContentResolver().openInputStream(uri)){return read(in,max);}}
    private static byte[] read(InputStream in,int max)throws Exception{ByteArrayOutputStream out=new ByteArrayOutputStream();byte[] b=new byte[8192];int n,total=0;while((n=in.read(b))!=-1){total+=n;if(total>max)throw new IOException("文件过大");out.write(b,0,n);}if(total==0)throw new IOException("文件为空");return out.toByteArray();}
    private static void write(File f,byte[] data)throws Exception{try(FileOutputStream out=new FileOutputStream(f)){out.write(data);out.getFD().sync();}}
    private String name(Uri uri){try(android.database.Cursor c=getContentResolver().query(uri,null,null,null,null)){if(c!=null&&c.moveToFirst()){int i=c.getColumnIndex(OpenableColumns.DISPLAY_NAME);if(i>=0)return c.getString(i);}}return uri.toString();}
    private EditText field(String value){EditText e=new EditText(this);e.setText(value);return e;}
    private TextView text(String value,int size){TextView t=new TextView(this);t.setText(value);t.setTextSize(size);t.setPadding(0,10,0,10);return t;}
    private void add(LinearLayout root,String label,View view){root.addView(text(label,14));root.addView(view);}
    private void toast(String value){Toast.makeText(this,value,Toast.LENGTH_LONG).show();}
    private void show(String value){runOnUiThread(()->output.setText(value));}
}
