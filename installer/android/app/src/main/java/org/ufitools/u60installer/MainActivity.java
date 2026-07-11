// SPDX-License-Identifier: AGPL-3.0-or-later
package org.ufitools.u60installer;

import android.app.Activity;
import android.content.Intent;
import android.net.Uri;
import android.os.Bundle;
import android.provider.OpenableColumns;
import android.view.View;
import android.widget.*;
import java.io.*;
import java.net.*;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.text.SimpleDateFormat;
import java.util.*;
import java.util.concurrent.Executors;

public final class MainActivity extends Activity {
    private static final int PICK = 7;
    private final java.util.concurrent.ExecutorService executor = Executors.newSingleThreadExecutor();
    private EditText base, session, confirmation;
    private CheckBox execute;
    private TextView selected, output;
    private Uri container;

    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        LinearLayout root = new LinearLayout(this);
        root.setOrientation(LinearLayout.VERTICAL); root.setPadding(32, 32, 32, 32);
        TextView title = text("U60 Pro 开源安装器", 24); root.addView(title);
        root.addView(text("默认只预览。执行时会先保存原厂备份，失败后自动触发回滚。", 15));
        base = field("http://192.168.0.1"); add(root, "设备管理地址", base);
        session = field(""); add(root, "UBUS 会话（无登录时留空）", session);
        Button choose = new Button(this); choose.setText("选择 backup_restore.bin"); choose.setOnClickListener(v -> pick()); root.addView(choose);
        selected = text("尚未选择文件", 14); root.addView(selected);
        execute = new CheckBox(this); execute.setText("执行设备恢复（不勾选则只预览）"); root.addView(execute);
        confirmation = field(""); add(root, "执行确认：输入 RESTORE U60 PRO", confirmation);
        Button start = new Button(this); start.setText("开始"); start.setOnClickListener(v -> start()); root.addView(start);
        output = text("等待操作", 13); output.setTextIsSelectable(true);
        ScrollView scroll = new ScrollView(this); scroll.addView(output); root.addView(scroll, new LinearLayout.LayoutParams(-1, 0, 1));
        setContentView(root);
    }

    private void pick() { startActivityForResult(new Intent(Intent.ACTION_OPEN_DOCUMENT).setType("application/octet-stream").addCategory(Intent.CATEGORY_OPENABLE), PICK); }
    @Override protected void onActivityResult(int request, int result, Intent data) {
        super.onActivityResult(request, result, data);
        if (request == PICK && result == RESULT_OK && data != null) { container = data.getData(); selected.setText(name(container)); }
    }
    private void start() {
        if (container == null) { toast("请先选择恢复容器"); return; }
        boolean mutate = execute.isChecked();
        if (mutate && !"RESTORE U60 PRO".equals(confirmation.getText().toString())) { toast("执行确认不匹配"); return; }
        String targetBase = base.getText().toString().trim();
        String targetSession = session.getText().toString().trim();
        output.setText(mutate ? "处理中，请勿断电…" : "正在验证预览…");
        executor.execute(() -> {
            try {
                byte[] candidate = read(container, 128 << 20);
                Installer installer = new Installer(targetBase, targetSession);
                if (!mutate) { show("预览通过\n容器大小: " + candidate.length + " 字节\n目标: " + installer.origin); return; }
                File backup = new File(getFilesDir(), "back_parameter-" + new SimpleDateFormat("yyyyMMdd-HHmmss", Locale.US).format(new Date()) + ".bin");
                byte[] original = installer.backup(); write(backup, original);
                installer.upload(candidate); installer.restore();
                if (installer.waitRestart()) { show("安装成功\n原厂备份: " + backup.getAbsolutePath()); return; }
                installer.retryUpload(original); installer.retryRestore();
                show("候选服务未恢复健康，已触发原厂备份回滚\n备份: " + backup.getAbsolutePath());
            } catch (Exception e) { show("失败: " + e.getMessage()); }
        });
    }

    private final class Installer {
        final String origin, sid;
        Installer(String raw, String session) throws Exception {
            URL url = new URL(raw);
            if (!("http".equals(url.getProtocol()) || "https".equals(url.getProtocol())) || url.getUserInfo() != null || !privateHost(url.getHost())) throw new IOException("仅允许私有/回环设备地址");
            origin = url.getProtocol() + "://" + url.getAuthority();
            sid = session.isEmpty() ? "00000000000000000000000000000000" : session;
            if (!sid.matches("[0-9a-fA-F]{32}")) throw new IOException("UBUS 会话必须是 32 位十六进制");
        }
        byte[] backup() throws Exception { rpc("device_backup_proc"); return request("GET", "/backup/back_parameter", null, null, 128 << 20); }
        void restore() throws Exception { rpc("device_restore_proc"); }
        void rpc(String procedure) throws Exception {
            String body = "[{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"call\",\"params\":[\"" + sid + "\",\"zwrt_mc.device.manager\",\"" + procedure + "\",{\"procType\":\"web\"}]}]";
            String response = new String(request("POST", "/ubus/?t=" + System.currentTimeMillis(), body.getBytes(StandardCharsets.UTF_8), "application/json", 1 << 20), StandardCharsets.UTF_8);
            if (!(response.contains("\"result\":[0") && response.toLowerCase(Locale.ROOT).contains("success"))) throw new IOException("UBUS 操作失败");
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
            if(body!=null){c.setDoOutput(true);c.setRequestProperty("Content-Type",contentType);try(OutputStream out=c.getOutputStream()){out.write(body);}}
            int status=c.getResponseCode(); if(status<200||status>=300)throw new IOException("HTTP "+status);
            try(InputStream in=c.getInputStream()){return read(in,max);} finally {c.disconnect();}
        }
    }

    private interface Action { void run() throws Exception; }
    private static boolean privateHost(String host) { try { InetAddress a=InetAddress.getByName(host); return a.isAnyLocalAddress()||a.isLoopbackAddress()||a.isSiteLocalAddress()||a.isLinkLocalAddress(); } catch(Exception e){return false;} }
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
