package com.shadowstar.vpn;

import android.app.Activity;
import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.Context;
import android.content.Intent;
import android.net.VpnService;
import android.os.Bundle;
import android.webkit.JavascriptInterface;
import android.webkit.WebChromeClient;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.Toast;

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

        // Start background Go daemon if native library exists
        startGoDaemonAsync();

        webView = new WebView(this);
        webView.setBackgroundColor(0xFF090D16);

        WebSettings settings = webView.getSettings();
        settings.setJavaScriptEnabled(true);
        settings.setDomStorageEnabled(true);
        settings.setDatabaseEnabled(true);
        settings.setAllowFileAccess(true);
        settings.setAllowContentAccess(true);
        settings.setCacheMode(WebSettings.LOAD_DEFAULT);
        settings.setUseWideViewPort(true);
        settings.setLoadWithOverviewMode(true);

        webView.setWebChromeClient(new WebChromeClient());
        webView.setWebViewClient(new WebViewClient());

        webView.addJavascriptInterface(new AndroidBridge(this), "AndroidBridge");

        setContentView(webView);

        // Load the embedded mobile interface
        webView.loadUrl("file:///android_asset/web/index.html");
    }

    private void startGoDaemonAsync() {
        new Thread(() -> {
            try {
                File binFile = new File(getApplicationInfo().nativeLibraryDir, "libshadowstar.so");
                if (!binFile.exists()) {
                    File target = new File(getFilesDir(), "shadowstar-daemon");
                    if (!target.exists()) {
                        InputStream in = getAssets().open("web/app.js");
                        in.close();
                    }
                    binFile = target;
                }

                if (binFile.exists()) {
                    binFile.setExecutable(true);
                    File cfgFile = new File(getFilesDir(), "config.json");
                    if (!cfgFile.exists()) {
                        copyAssetToFile("web/shadowstar-client.json", cfgFile);
                    }
                    ProcessBuilder pb = new ProcessBuilder(binFile.getAbsolutePath(), "-c", cfgFile.getAbsolutePath());
                    pb.directory(getFilesDir());
                    daemonProcess = pb.start();
                }
            } catch (Exception ignored) {
            }
        }).start();
    }

    private void copyAssetToFile(String assetName, File dest) {
        try (InputStream in = getAssets().open(assetName);
             OutputStream out = new FileOutputStream(dest)) {
            byte[] buf = new byte[8192];
            int n;
            while ((n = in.read(buf)) > 0) {
                out.write(buf, 0, n);
            }
        } catch (Exception ignored) {
        }
    }

    @Override
    protected void onActivityResult(int requestCode, int resultCode, Intent data) {
        super.onActivityResult(requestCode, resultCode, data);
        if (requestCode == VPN_REQUEST_CODE) {
            if (resultCode == RESULT_OK) {
                Intent vpnIntent = new Intent(this, ShadowStarVpnService.class);
                vpnIntent.setAction("START");
                startService(vpnIntent);
                Toast.makeText(this, "✓ ShadowStar VPN запущен", Toast.LENGTH_SHORT).show();
            } else {
                Toast.makeText(this, "Требуется разрешение для VPN-туннеля", Toast.LENGTH_SHORT).show();
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
            daemonProcess.destroy();
        }
    }

    public static class AndroidBridge {
        private final MainActivity activity;

        public AndroidBridge(MainActivity activity) {
            this.activity = activity;
        }

        @JavascriptInterface
        public void startVpn() {
            activity.runOnUiThread(() -> {
                Intent prepare = VpnService.prepare(activity);
                if (prepare != null) {
                    activity.startActivityForResult(prepare, VPN_REQUEST_CODE);
                } else {
                    Intent vpnIntent = new Intent(activity, ShadowStarVpnService.class);
                    vpnIntent.setAction("START");
                    activity.startService(vpnIntent);
                }
            });
        }

        @JavascriptInterface
        public void stopVpn() {
            activity.runOnUiThread(() -> {
                Intent vpnIntent = new Intent(activity, ShadowStarVpnService.class);
                vpnIntent.setAction("STOP");
                activity.startService(vpnIntent);
            });
        }

        @JavascriptInterface
        public void copyToClipboard(String text) {
            activity.runOnUiThread(() -> {
                ClipboardManager cm = (ClipboardManager) activity.getSystemService(Context.CLIPBOARD_SERVICE);
                if (cm != null) {
                    cm.setPrimaryClip(ClipData.newPlainText("shadowstar", text));
                    Toast.makeText(activity, "✓ Скопировано в буфер!", Toast.LENGTH_SHORT).show();
                }
            });
        }

        @JavascriptInterface
        public void showToast(String msg) {
            activity.runOnUiThread(() -> Toast.makeText(activity, msg, Toast.LENGTH_SHORT).show());
        }

        @JavascriptInterface
        public String getPlatform() {
            return "android";
        }
    }
}
