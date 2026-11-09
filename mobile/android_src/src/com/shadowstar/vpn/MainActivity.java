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

import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.FileOutputStream;
import java.io.InputStream;
import java.io.OutputStream;

public class MainActivity extends Activity {

    private static final int VPN_REQUEST_CODE = 0x55;
    private WebView webView;
    private Process daemonProcess;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);

        startGoDaemonAsync();

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

    private void startGoDaemonAsync() {
        Thread thread = new Thread(new DaemonRunner(this));
        thread.start();
    }

    private static class DaemonRunner implements Runnable {
        private final MainActivity activity;

        DaemonRunner(MainActivity activity) {
            this.activity = activity;
        }

        @Override
        public void run() {
            try {
                File binFile = new File(activity.getApplicationInfo().nativeLibraryDir, "libshadowstar.so");
                if (binFile.exists()) {
                    binFile.setExecutable(true);
                    File cfgFile = new File(activity.getFilesDir(), "config.json");
                    if (!cfgFile.exists()) {
                        activity.copyAssetToFile("config.json", cfgFile);
                    }
                    ProcessBuilder pb = new ProcessBuilder(binFile.getAbsolutePath(), "-c", cfgFile.getAbsolutePath());
                    pb.directory(activity.getFilesDir());
                    activity.daemonProcess = pb.start();
                }
            } catch (Throwable ignored) {
            }
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
                try {
                    Intent vpnIntent = new Intent(this, ShadowStarVpnService.class);
                    vpnIntent.setAction("START");
                    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                        startForegroundService(vpnIntent);
                    } else {
                        startService(vpnIntent);
                    }
                } catch (Throwable t) {
                    try {
                        Intent vpnIntent = new Intent(this, ShadowStarVpnService.class);
                        vpnIntent.setAction("START");
                        startService(vpnIntent);
                    } catch (Throwable ignored) {}
                }
                Toast.makeText(this, "✓ ShadowStar VPN запущен", Toast.LENGTH_SHORT).show();
                if (webView != null) {
                    webView.evaluateJavascript("if(window.onVpnStatusChanged) window.onVpnStatusChanged(true);", null);
                }
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
        if (daemonProcess != null) {
            try {
                daemonProcess.destroy();
            } catch (Throwable ignored) {}
        }
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
            activity.runOnUiThread(new StartVpnRunnable(activity));
        }

        @JavascriptInterface
        public void stopVpn() {
            activity.runOnUiThread(new StopVpnRunnable(activity));
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
            final java.util.concurrent.CountDownLatch latch = new java.util.concurrent.CountDownLatch(1);
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
                latch.await(350, java.util.concurrent.TimeUnit.MILLISECONDS);
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

    private static class StartVpnRunnable implements Runnable {
        private final MainActivity activity;

        StartVpnRunnable(MainActivity activity) {
            this.activity = activity;
        }

        @Override
        public void run() {
            try {
                Intent prepare = VpnService.prepare(activity);
                if (prepare != null) {
                    activity.startActivityForResult(prepare, VPN_REQUEST_CODE);
                } else {
                    Intent vpnIntent = new Intent(activity, ShadowStarVpnService.class);
                    vpnIntent.setAction("START");
                    try {
                        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                            activity.startForegroundService(vpnIntent);
                        } else {
                            activity.startService(vpnIntent);
                        }
                    } catch (Throwable t) {
                        try {
                            activity.startService(vpnIntent);
                        } catch (Throwable ignored) {}
                    }
                    if (activity.webView != null) {
                        activity.webView.evaluateJavascript("if(window.onVpnStatusChanged) window.onVpnStatusChanged(true);", null);
                    }
                }
            } catch (Throwable t) {
                try {
                    Toast.makeText(activity, "Ошибка запуска: " + t.getMessage(), Toast.LENGTH_LONG).show();
                } catch (Throwable ignored) {}
            }
        }
    }

    private static class StopVpnRunnable implements Runnable {
        private final MainActivity activity;

        StopVpnRunnable(MainActivity activity) {
            this.activity = activity;
        }

        @Override
        public void run() {
            try {
                Intent vpnIntent = new Intent(activity, ShadowStarVpnService.class);
                vpnIntent.setAction("STOP");
                try {
                    activity.startService(vpnIntent);
                } catch (Throwable ignored) {}
                if (activity.webView != null) {
                    activity.webView.evaluateJavascript("if(window.onVpnStatusChanged) window.onVpnStatusChanged(false);", null);
                }
            } catch (Throwable ignored) {}
        }
    }
}
