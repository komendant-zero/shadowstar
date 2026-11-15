package com.shadowstar.vpn;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.content.Intent;
import android.net.ProxyInfo;
import android.net.VpnService;
import android.os.Build;
import android.os.ParcelFileDescriptor;

import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.net.DatagramPacket;
import java.net.DatagramSocket;
import java.net.InetAddress;

public class ShadowStarVpnService extends VpnService {

    private static final String CHANNEL_ID = "shadowstar_vpn_channel";
    private ParcelFileDescriptor vpnInterface;
    private DatagramSocket relaySocket;
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
            }

            Builder builder = new Builder();
            builder.setSession("ShadowStar");
            builder.addAddress("10.254.1.2", 24);

            try {
                builder.addDisallowedApplication(getPackageName());
                android.content.SharedPreferences prefs = getSharedPreferences("shadowstar_prefs", MODE_PRIVATE);

                // Split tunneling: disallowed packages configured explicitly by user
                java.util.Set<String> disallowed = prefs.getStringSet("disallowed_packages", null);
                if (disallowed != null) {
                    for (String pkg : disallowed) {
                        try {
                            builder.addDisallowedApplication(pkg);
                        } catch (Throwable ignored) {}
                    }
                }
            } catch (Throwable ignored) {
            }

            // Transparent L3 TUN routing (no HTTP proxy set, ensuring games like Billionaire Casino do not trigger anti-cheat proxy blocks)
            builder.addRoute("0.0.0.0", 0);
            builder.addDnsServer("1.1.1.1");
            builder.addDnsServer("8.8.8.8");
            builder.setMtu(1500);
            builder.setBlocking(true);

            vpnInterface = builder.establish();
            ShadowStarTileService.isVpnActive = true;

            if (vpnInterface != null) {
                try {
                    relaySocket = new DatagramSocket();
                    try {
                        protect(relaySocket);
                    } catch (Throwable ignored) {}
                    try {
                        relaySocket.setReceiveBufferSize(2 * 1024 * 1024);
                        relaySocket.setSendBufferSize(2 * 1024 * 1024);
                    } catch (Throwable ignored) {}

                    final DatagramSocket sock = relaySocket;
                    final InetAddress loopback = InetAddress.getByName("127.0.0.1");
                    final int relayPort = 10811;

                    // Thread 1: Read L3 IP packets from Android TUN -> forward to Go NetStack
                    new Thread(() -> {
                        try (FileInputStream in = new FileInputStream(vpnInterface.getFileDescriptor())) {
                            byte[] packet = new byte[32768];
                            while (isRunning && vpnInterface != null && !sock.isClosed()) {
                                try {
                                    int len = in.read(packet);
                                    if (len <= 0) continue;
                                    DatagramPacket dp = new DatagramPacket(packet, len, loopback, relayPort);
                                    sock.send(dp);
                                } catch (Throwable t) {
                                    if (!isRunning) break;
                                    try { Thread.sleep(20); } catch (InterruptedException ignored) {}
                                }
                            }
                        } catch (Throwable ignored) {
                        }
                    }, "ShadowStar-TunReader").start();

                    // Thread 2: Read processed response IP packets from Go NetStack -> inject into Android TUN
                    new Thread(() -> {
                        try (FileOutputStream out = new FileOutputStream(vpnInterface.getFileDescriptor())) {
                            byte[] respBuf = new byte[32768];
                            DatagramPacket respPkt = new DatagramPacket(respBuf, respBuf.length);
                            while (isRunning && vpnInterface != null && !sock.isClosed()) {
                                try {
                                    // CRITICAL: DatagramPacket.getLength() mutates to last received packet length.
                                    // Resetting buffer capacity prevents truncation of larger game/data packets!
                                    respPkt.setLength(respBuf.length);
                                    sock.receive(respPkt);
                                    int len = respPkt.getLength();
                                    if (len > 0) {
                                        out.write(respPkt.getData(), 0, len);
                                    }
                                } catch (Throwable t) {
                                    if (!isRunning) break;
                                    try { Thread.sleep(20); } catch (InterruptedException ignored) {}
                                }
                            }
                        } catch (Throwable ignored) {
                        }
                    }, "ShadowStar-TunWriter").start();

                } catch (Throwable t) {
                    // Fallback packet drainer
                    new Thread(() -> {
                        try (FileInputStream in = new FileInputStream(vpnInterface.getFileDescriptor())) {
                            byte[] packet = new byte[32768];
                            while (isRunning && vpnInterface != null) {
                                int len = in.read(packet);
                                if (len <= 0) break;
                            }
                        } catch (Throwable ignored) {}
                    }).start();
                }
            }
        } catch (Throwable t) {
            stopVpn();
        }
    }

    private void stopVpn() {
        isRunning = false;
        ShadowStarTileService.isVpnActive = false;
        if (relaySocket != null) {
            try {
                relaySocket.close();
            } catch (Throwable ignored) {}
            relaySocket = null;
        }
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
