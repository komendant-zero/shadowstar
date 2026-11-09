package com.shadowstar.vpn;

import android.app.Activity;
import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.Context;
import android.content.Intent;
import android.net.VpnService;
import android.os.Build;
import android.os.Bundle;
import android.webkit.JavascriptInterface;
import android.webkit.WebChromeClient;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.Toast;

import java.io.BufferedReader;
import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.FileOutputStream;
import java.io.InputStream;
import java.io.InputStreamReader;
import java.io.OutputStream;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;

public class MainActivity extends Activity {

    private static final int VPN_REQUEST_CODE = 0x55;
    private WebView webView;
    private Process daemonProcess;

    public String lastProfileName = "";
    public String lastProfileAddr = "";
    public String lastProfileKey = "";
    public String lastProfileSni = "www.microsoft.com";

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);

        initDefaultProfileFromAsset();
        startGoDaemonInitial();

        webView = new WebView(this);
        webView.setBackgroundColor(0xFF090D16);

        WebSettings settings = webView.getSettings();
        settings.setJavaScriptEnabled(true);
        settings.setDomStorageEnabled(true);
        settings.setDatabaseEnabled(true);
        settings.setAllowFileAccess(true);
        settings.setAllowContentAccess(true);
        settings.setAllowFileAccessFromFileURLs(true);
        settings.setAllowUniversalAccessFromFileURLs(true);
        settings.setCacheMode(WebSettings.LOAD_DEFAULT);
        settings.setUseWideViewPort(true);
        settings.setLoadWithOverviewMode(true);

        webView.setWebChromeClient(new WebChromeClient());
        webView.setWebViewClient(new SafeWebViewClient(this));

        webView.addJavascriptInterface(new AndroidBridge(this), "AndroidBridge");
        setContentView(webView);

        loadEmbeddedHtml();
    }

    public void loadEmbeddedHtml() {
        String html = loadAsset("index.html");
        if (html != null && !html.isEmpty()) {
            webView.loadDataWithBaseURL("https://shadowstar.app/", html, "text/html", "UTF-8", null);
        } else {
            String fallback = "<!DOCTYPE html><html><head><meta charset='UTF-8'><meta name='viewport' content='width=device-width, initial-scale=1.0'>"
                    + "<style>body{background:#090D16;color:#FFF;font-family:sans-serif;padding:24px;text-align:center;}</style></head>"
                    + "<body><h2>⭐ SHADOWSTAR</h2><p>Загрузка интерфейса...</p></body></html>";
            webView.loadDataWithBaseURL("https://shadowstar.app/", fallback, "text/html", "UTF-8", null);
        }
    }

    private void initDefaultProfileFromAsset() {
        try {
            String cfg = loadAsset("config.json");
            if (cfg != null && !cfg.isEmpty()) {
                org.json.JSONObject obj = new org.json.JSONObject(cfg);
                org.json.JSONArray servers = obj.optJSONArray("servers");
                if (servers != null && servers.length() > 0) {
                    org.json.JSONObject first = servers.getJSONObject(0);
                    lastProfileName = first.optString("name", "ShadowStar Node");
                    lastProfileAddr = first.optString("server_addr", "demo-node.shadowstar.org:4433");
                    lastProfileKey = first.optString("master_key", "shadowstar-demo-key-v1");
                    lastProfileSni = first.optString("sni", "www.microsoft.com");
                }
            }
        } catch (Throwable ignored) {}
    }

    public String loadAsset(String name) {
        String[] candidates = new String[]{name, "web/" + name, "web\\" + name};
        for (String candidate : candidates) {
            try (InputStream in = getAssets().open(candidate);
                 ByteArrayOutputStream out = new ByteArrayOutputStream()) {
                byte[] buf = new byte[8192];
                int n;
                while ((n = in.read(buf)) > 0) {
                    out.write(buf, 0, n);
                }
                return out.toString("UTF-8");
            } catch (Throwable ignored) {
            }
        }
        return "";
    }

    private void startGoDaemonInitial() {
        new Thread(() -> {
            try {
                File binFile = new File(getApplicationInfo().nativeLibraryDir, "libshadowstar.so");
                if (binFile.exists()) {
                    binFile.setExecutable(true);
                    File cfgFile = new File(getFilesDir(), "config.json");
                    if (!cfgFile.exists()) {
                        copyAssetToFile("config.json", cfgFile);
                    }
                    ProcessBuilder pb = new ProcessBuilder(binFile.getAbsolutePath(), "-c", cfgFile.getAbsolutePath());
                    pb.directory(getFilesDir());
                    pb.redirectErrorStream(true);
                    daemonProcess = pb.start();
                    drainProcessOutput(daemonProcess);
                }
            } catch (Throwable ignored) {
            }
        }).start();
    }

    public void startGoDaemonWithProfile(String addr, String key, String sni) {
        new Thread(() -> {
            try {
                if (daemonProcess != null) {
                    try { daemonProcess.destroy(); } catch (Throwable ignored) {}
                    daemonProcess = null;
                }

                File binFile = new File(getApplicationInfo().nativeLibraryDir, "libshadowstar.so");
                if (binFile.exists()) {
                    binFile.setExecutable(true);

                    String cleanKey = (key != null && !key.isEmpty()) ? key : lastProfileKey;
                    String cleanAddr = (addr != null && !addr.isEmpty()) ? addr : lastProfileAddr;
                    String cleanSni = (sni != null && !sni.isEmpty()) ? sni : lastProfileSni;

                    File cfgFile = new File(getFilesDir(), "config.json");
                    String cfgJson = "{\"socks_addr\":\"127.0.0.1:10808\",\"http_addr\":\"127.0.0.1:10809\","
                            + "\"servers\":[{\"id\":\"active\",\"name\":\"Active Node\","
                            + "\"server_addr\":\"" + cleanAddr + "\","
                            + "\"master_key\":\"" + cleanKey + "\","
                            + "\"sni\":\"" + cleanSni + "\","
                            + "\"enable_chunker\":true}],"
                            + "\"active_id\":\"active\"}";
                    try (FileOutputStream fos = new FileOutputStream(cfgFile)) {
                        fos.write(cfgJson.getBytes("UTF-8"));
                    } catch (Throwable ignored) {}

                    String uri = "shadowstar://" + cleanKey + "@" + cleanAddr + "/?sni=" + cleanSni + "&chunker=1&name=Active";
                    ProcessBuilder pb = new ProcessBuilder(
                            binFile.getAbsolutePath(),
                            "-c", cfgFile.getAbsolutePath(),
                            "-connect", uri
                    );
                    pb.directory(getFilesDir());
                    pb.redirectErrorStream(true);
                    daemonProcess = pb.start();
                    drainProcessOutput(daemonProcess);
                }
            } catch (Throwable ignored) {
            }
        }).start();
    }

    private void drainProcessOutput(Process process) {
        new Thread(() -> {
            try (BufferedReader reader = new BufferedReader(new InputStreamReader(process.getInputStream()))) {
                String line;
                while ((line = reader.readLine()) != null) {
                    android.util.Log.i("ShadowStarDaemon", line);
                }
            } catch (Throwable ignored) {}
        }).start();
    }

    public void launchVpnService() {
        startGoDaemonWithProfile(lastProfileAddr, lastProfileKey, lastProfileSni);

        Intent vpnIntent = new Intent(this, ShadowStarVpnService.class);
        vpnIntent.setAction("START");
        try {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                startForegroundService(vpnIntent);
            } else {
                startService(vpnIntent);
            }
        } catch (Throwable t) {
            try {
                startService(vpnIntent);
            } catch (Throwable ignored) {}
        }

        if (webView != null) {
            webView.evaluateJavascript("if(window.onVpnStatusChanged) window.onVpnStatusChanged(true);", null);
        }
    }

    public void stopVpnService() {
        if (daemonProcess != null) {
            try { daemonProcess.destroy(); } catch (Throwable ignored) {}
            daemonProcess = null;
        }

        Intent vpnIntent = new Intent(this, ShadowStarVpnService.class);
        vpnIntent.setAction("STOP");
        try {
            startService(vpnIntent);
        } catch (Throwable ignored) {}

        if (webView != null) {
            webView.evaluateJavascript("if(window.onVpnStatusChanged) window.onVpnStatusChanged(false);", null);
        }
    }

    public void copyAssetToFile(String assetName, File dest) {
        String[] candidates = new String[]{assetName, "web/" + assetName, "web\\" + assetName};
        for (String candidate : candidates) {
            try (InputStream in = getAssets().open(candidate);
                 OutputStream out = new FileOutputStream(dest)) {
                byte[] buf = new byte[8192];
                int n;
                while ((n = in.read(buf)) > 0) {
                    out.write(buf, 0, n);
                }
                return;
            } catch (Throwable ignored) {
            }
        }
    }

    @Override
    protected void onActivityResult(int requestCode, int resultCode, Intent data) {
        super.onActivityResult(requestCode, resultCode, data);
        if (requestCode == VPN_REQUEST_CODE) {
            if (resultCode == RESULT_OK) {
                launchVpnService();
                Toast.makeText(this, "ShadowStar VPN запущен", Toast.LENGTH_SHORT).show();
            } else {
                Toast.makeText(this, "Требуется разрешение для VPN-туннеля", Toast.LENGTH_SHORT).show();
                if (webView != null) {
                    webView.evaluateJavascript("if(window.onVpnStatusChanged) window.onVpnStatusChanged(false);", null);
                }
            }
        }
    }

    @Override
    public void onBackPressed() {
        if (webView != null && webView.canGoBack()) {
            webView.goBack();
        } else {
            super.onBackPressed();
        }
    }

    @Override
    protected void onDestroy() {
        super.onDestroy();
        stopVpnService();
    }

    private static class SafeWebViewClient extends WebViewClient {
        private final MainActivity activity;

        SafeWebViewClient(MainActivity activity) {
            this.activity = activity;
        }

        @Override
        public void onReceivedError(WebView view, int errorCode, String description, String failingUrl) {
            if (activity != null) {
                activity.loadEmbeddedHtml();
            }
        }
    }

    public static class AndroidBridge {
        private final MainActivity activity;

        public AndroidBridge(MainActivity activity) {
            this.activity = activity;
        }

        @JavascriptInterface
        public void startVpn() {
            startVpnWithProfile(activity.lastProfileName, activity.lastProfileAddr, activity.lastProfileKey, activity.lastProfileSni);
        }

        @JavascriptInterface
        public void startVpnWithProfile(String name, String addr, String key, String sni) {
            activity.runOnUiThread(() -> {
                if (name != null && !name.isEmpty()) activity.lastProfileName = name;
                if (addr != null && !addr.isEmpty()) activity.lastProfileAddr = addr;
                if (key != null && !key.isEmpty()) activity.lastProfileKey = key;
                if (sni != null && !sni.isEmpty()) activity.lastProfileSni = sni;

                Intent prepare = VpnService.prepare(activity);
                if (prepare != null) {
                    activity.startActivityForResult(prepare, VPN_REQUEST_CODE);
                } else {
                    activity.launchVpnService();
                }
            });
        }

        @JavascriptInterface
        public void stopVpn() {
            activity.runOnUiThread(activity::stopVpnService);
        }

        @JavascriptInterface
        public String getInitialConfig() {
            return activity.loadAsset("config.json");
        }

        @JavascriptInterface
        public void copyToClipboard(String text) {
            activity.runOnUiThread(() -> {
                try {
                    ClipboardManager cm = (ClipboardManager) activity.getSystemService(Context.CLIPBOARD_SERVICE);
                    if (cm != null) {
                        cm.setPrimaryClip(ClipData.newPlainText("shadowstar", text));
                        Toast.makeText(activity, "Скопировано в буфер", Toast.LENGTH_SHORT).show();
                    }
                } catch (Throwable ignored) {}
            });
        }

        @JavascriptInterface
        public String getClipboardText() {
            final String[] result = new String[]{""};
            final CountDownLatch latch = new CountDownLatch(1);
            activity.runOnUiThread(() -> {
                try {
                    ClipboardManager cm = (ClipboardManager) activity.getSystemService(Context.CLIPBOARD_SERVICE);
                    if (cm != null && cm.hasPrimaryClip() && cm.getPrimaryClip().getItemCount() > 0) {
                        CharSequence cs = cm.getPrimaryClip().getItemAt(0).getText();
                        if (cs != null) {
                            result[0] = cs.toString();
                        }
                    }
                } catch (Throwable ignored) {
                } finally {
                    latch.countDown();
                }
            });
            try {
                latch.await(350, TimeUnit.MILLISECONDS);
            } catch (InterruptedException ignored) {}
            return result[0];
        }

        @JavascriptInterface
        public void showToast(String msg) {
            activity.runOnUiThread(() -> {
                try {
                    Toast.makeText(activity, msg, Toast.LENGTH_SHORT).show();
                } catch (Throwable ignored) {}
            });
        }

        @JavascriptInterface
        public String getPlatform() {
            return "android";
        }
    }
}
