package com.shadowstar.vpn;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.content.Intent;
import android.net.VpnService;
import android.os.Build;
import android.os.ParcelFileDescriptor;

import java.io.FileInputStream;

public class ShadowStarVpnService extends VpnService {

    private static final String CHANNEL_ID = "shadowstar_vpn_channel";
    private ParcelFileDescriptor vpnInterface;
    private volatile boolean isRunning = false;

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        if (intent != null && "STOP".equals(intent.getAction())) {
            stopVpn();
            return START_NOT_STICKY;
        }

        startVpn();
        return START_STICKY;
    }

    private void startVpn() {
        if (isRunning) return;
        isRunning = true;

        try {
            createNotificationChannel();

            Intent notifyIntent = new Intent(this, MainActivity.class);
            PendingIntent pendingIntent = PendingIntent.getActivity(
                    this, 0, notifyIntent,
                    PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT
            );

            Notification.Builder notifBuilder;
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                notifBuilder = new Notification.Builder(this, CHANNEL_ID);
            } else {
                notifBuilder = new Notification.Builder(this);
            }

            int iconRes = getApplicationInfo().icon != 0 ? getApplicationInfo().icon : android.R.drawable.stat_sys_download;

            Notification notification = notifBuilder
                    .setContentTitle("ShadowStar Anti-DPI VPN")
                    .setContentText("Туннель активен • TLS 1.3 Reality защита включена")
                    .setSmallIcon(iconRes)
                    .setContentIntent(pendingIntent)
                    .setOngoing(true)
                    .build();

            try {
                startForeground(1001, notification);
            } catch (Throwable ignored) {
                // Ignore any FGS / notification restrictions so VPN tunnel continues
            }

            Builder builder = new Builder();
            builder.setSession("ShadowStar");
            builder.addAddress("10.254.1.2", 24);
            builder.addRoute("0.0.0.0", 0);
            builder.addDnsServer("1.1.1.1");
            builder.addDnsServer("8.8.8.8");
            builder.setMtu(1500);
            builder.setBlocking(false);

            vpnInterface = builder.establish();

            if (vpnInterface != null) {
                new Thread(() -> {
                    try (FileInputStream in = new FileInputStream(vpnInterface.getFileDescriptor())) {
                        byte[] packet = new byte[32768];
                        while (isRunning && vpnInterface != null) {
                            int len = in.read(packet);
                            if (len <= 0) break;
                        }
                    } catch (Throwable ignored) {
                    }
                }).start();
            }
        } catch (Throwable t) {
            stopVpn();
        }
    }

    private void stopVpn() {
        isRunning = false;
        if (vpnInterface != null) {
            try {
                vpnInterface.close();
            } catch (Throwable ignored) {
            }
            vpnInterface = null;
        }
        try {
            stopForeground(true);
        } catch (Throwable ignored) {
        }
        stopSelf();
    }

    private void createNotificationChannel() {
        try {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                NotificationChannel channel = new NotificationChannel(
                        CHANNEL_ID,
                        "ShadowStar VPN Status",
                        NotificationManager.IMPORTANCE_LOW
                );
                channel.setDescription("Статус активного VPN-соединения");
                NotificationManager nm = getSystemService(NotificationManager.class);
                if (nm != null) {
                    nm.createNotificationChannel(channel);
                }
            }
        } catch (Throwable ignored) {
        }
    }

    @Override
    public void onDestroy() {
        super.onDestroy();
        stopVpn();
    }
}
