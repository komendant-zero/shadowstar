package com.shadowstar.vpn

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.os.ParcelFileDescriptor
import android.util.Log
import androidx.core.app.NotificationCompat
import kotlinx.coroutines.*
import java.io.File
import java.io.FileOutputStream
import java.net.HttpURLConnection
import java.net.URL

class ShadowStarVpnService : VpnService() {

    private var vpnInterface: ParcelFileDescriptor? = null
    private var coreProcess: Process? = null
    private val serviceScope = CoroutineScope(Dispatchers.IO + SupervisorJob())
    private var isRunning = false

    companion object {
        const val ACTION_CONNECT = "com.shadowstar.vpn.CONNECT"
        const val ACTION_DISCONNECT = "com.shadowstar.vpn.DISCONNECT"
        const val EXTRA_CONFIG_JSON = "EXTRA_CONFIG_JSON"
        const val NOTIFICATION_ID = 4433
        const val CHANNEL_ID = "shadowstar_vpn_channel"
        const val TAG = "ShadowStarVPN"

        var isServiceRunning = false
            private set
    }

    override fun onCreate() {
        super.onCreate()
        createNotificationChannel()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        val action = intent?.action ?: return START_NOT_STICKY

        when (action) {
            ACTION_CONNECT -> {
                val configJson = intent.getStringExtra(EXTRA_CONFIG_JSON) ?: ""
                startVpn(configJson)
            }
            ACTION_DISCONNECT -> {
                stopVpn()
            }
        }

        return START_STICKY
    }

    private fun startVpn(configJson: String) {
        if (isRunning) return
        isRunning = true
        isServiceRunning = true

        startForeground(NOTIFICATION_ID, buildNotification("Подключение к ShadowStar...", "Согласование TLS 1.3 Reality"))

        serviceScope.launch {
            try {
                // 1. Extract embedded native Go binary if needed
                val coreBinary = getOrExtractCoreBinary()

                // 2. Launch ShadowStar Go Mobile Daemon in background
                val processBuilder = ProcessBuilder(
                    coreBinary.absolutePath,
                    "-api=10810",
                    "-socks=127.0.0.1:10808",
                    "-http=127.0.0.1:10809"
                )
                processBuilder.directory(filesDir)
                coreProcess = processBuilder.start()

                // Wait 500ms for core to spin up
                delay(500)

                // 3. Send connect command to local REST API
                sendConnectCommand(configJson)

                // 4. Establish Android TUN Interface
                val builder = Builder()
                    .setSession("ShadowStar Stealth VPN")
                    .setMtu(1500)
                    .addAddress("10.0.0.2", 32)
                    .addRoute("0.0.0.0", 0) // Route all IPv4 traffic
                    .addDnsServer("1.1.1.1")
                    .addDnsServer("8.8.8.8")
                    .setBlocking(true)

                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                    builder.setMetered(false)
                }

                vpnInterface = builder.establish()
                Log.i(TAG, "VPN TUN interface established: fd=${vpnInterface?.fd}")

                // Update notification
                val notifManager = getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
                notifManager.notify(NOTIFICATION_ID, buildNotification("Анти-DPI туннель активен", "Защита TLS 1.3 Reality включена"))

                // Broadcast connected state
                sendBroadcast(Intent("com.shadowstar.vpn.STATE_CHANGED").apply {
                    putExtra("connected", true)
                })

            } catch (e: Exception) {
                Log.e(TAG, "VPN start failed: ${e.message}", e)
                stopVpn()
            }
        }
    }

    private fun stopVpn() {
        isRunning = false
        isServiceRunning = false

        serviceScope.launch {
            try {
                // Send disconnect to REST API
                val url = URL("http://127.0.0.1:10810/api/disconnect")
                val conn = url.openConnection() as HttpURLConnection
                conn.requestMethod = "POST"
                conn.connectTimeout = 1000
                conn.responseCode
                conn.disconnect()
            } catch (e: Exception) {
                // ignore
            }

            coreProcess?.destroy()
            coreProcess = null

            vpnInterface?.close()
            vpnInterface = null

            stopForeground(STOP_FOREGROUND_REMOVE)
            stopSelf()

            sendBroadcast(Intent("com.shadowstar.vpn.STATE_CHANGED").apply {
                putExtra("connected", false)
            })
        }
    }

    private fun sendConnectCommand(configJson: String) {
        val url = URL("http://127.0.0.1:10810/api/connect")
        val conn = url.openConnection() as HttpURLConnection
        conn.requestMethod = "POST"
        conn.doOutput = true
        conn.setRequestProperty("Content-Type", "application/json")
        conn.outputStream.use { os ->
            val payload = "{\"profile_json\":${JSONObject.quote(configJson)}}"
            os.write(payload.toByteArray())
        }
        val code = conn.responseCode
        conn.disconnect()
        Log.i(TAG, "Connect API responded: code=$code")
    }

    private fun getOrExtractCoreBinary(): File {
        val binaryFile = File(filesDir, "shadowstar-core")
        if (!binaryFile.exists() || binaryFile.length() == 0L) {
            val assetName = "shadowstar-mobile-android-arm64"
            assets.open(assetName).use { input ->
                FileOutputStream(binaryFile).use { output ->
                    input.copyTo(output)
                }
            }
            binaryFile.setExecutable(true, false)
        }
        return binaryFile
    }

    private fun createNotificationChannel() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val channel = NotificationChannel(
                CHANNEL_ID,
                "ShadowStar VPN Status",
                NotificationManager.IMPORTANCE_LOW
            ).apply {
                description = "Отображает статус активного VPN подключения"
                setShowBadge(false)
            }
            val manager = getSystemService(NotificationManager::class.java)
            manager.createNotificationChannel(channel)
        }
    }

    private fun buildNotification(title: String, text: String): Notification {
        val mainIntent = Intent(this, MainActivity::class.java)
        val pendingIntent = PendingIntent.getActivity(
            this, 0, mainIntent,
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT
        )

        val disconnectIntent = Intent(this, ShadowStarVpnService::class.java).apply {
            action = ACTION_DISCONNECT
        }
        val disconnectPending = PendingIntent.getService(
            this, 1, disconnectIntent,
            PendingIntent.FLAG_IMMUTABLE
        )

        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setContentTitle(title)
            .setContentText(text)
            .setSmallIcon(android.R.drawable.ic_dialog_info)
            .setContentIntent(pendingIntent)
            .addAction(android.R.drawable.ic_menu_close_clear_cancel, "Отключить", disconnectPending)
            .setOngoing(true)
            .setPriority(NotificationCompat.PRIORITY_LOW)
            .build()
    }

    override fun onDestroy() {
        serviceScope.cancel()
        super.onDestroy()
    }
}
