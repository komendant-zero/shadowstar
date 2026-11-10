package com.shadowstar.vpn

import android.app.Activity
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.net.VpnService
import android.os.Bundle
import android.widget.Toast
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import org.json.JSONArray
import org.json.JSONObject

class MainActivity : ComponentActivity() {

    private var isConnected by mutableStateOf(false)
    private var serversList by mutableStateOf(listOf<ServerItem>())
    private var activeServerIndex by mutableStateOf(0)
    private var speedDown by mutableStateOf("0 KB/s")
    private var speedUp by mutableStateOf("0 KB/s")

    private val vpnPermissionLauncher = registerForActivityResult(
        ActivityResultContracts.StartActivityForResult()
    ) { result ->
        if (result.resultCode == Activity.RESULT_OK) {
            startVpnService()
        } else {
            Toast.makeText(this, "Требуется разрешение на VPN", Toast.LENGTH_SHORT).show()
        }
    }

    private val vpnStateReceiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context?, intent: Intent?) {
            isConnected = intent?.getBooleanExtra("connected", false) ?: false
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        registerReceiver(vpnStateReceiver, IntentFilter("com.shadowstar.vpn.STATE_CHANGED"), RECEIVER_NOT_EXPORTED)

        // Load saved servers or default demo node
        loadServers()

        // Handle intent if launched via shadowstar:// URL
        intent?.data?.toString()?.let { uri ->
            if (uri.startsWith("shadowstar://")) {
                importFromUri(uri)
            }
        }

        setContent {
            ShadowStarMobileTheme {
                MainScreen(
                    isConnected = isConnected,
                    speedDown = speedDown,
                    speedUp = speedUp,
                    servers = serversList,
                    selectedIndex = activeServerIndex,
                    onToggleConnect = { toggleConnection() },
                    onSelectServer = { idx -> activeServerIndex = idx; saveServers() },
                    onDeleteServer = { idx -> deleteServer(idx) },
                    onAddServer = { s -> addServer(s) }
                )
            }
        }
    }

    private fun toggleConnection() {
        if (isConnected) {
            val intent = Intent(this, ShadowStarVpnService::class.java).apply {
                action = ShadowStarVpnService.ACTION_DISCONNECT
            }
            startService(intent)
        } else {
            if (serversList.isEmpty()) {
                Toast.makeText(this, "Добавьте хотя бы один узел!", Toast.LENGTH_SHORT).show()
                return
            }
            val vpnIntent = VpnService.prepare(this)
            if (vpnIntent != null) {
                vpnPermissionLauncher.launch(vpnIntent)
            } else {
                startVpnService()
            }
        }
    }

    private fun startVpnService() {
        if (serversList.isEmpty()) return
        val current = serversList.getOrNull(activeServerIndex) ?: serversList.first()
        val json = JSONObject().apply {
            put("id", current.id)
            put("name", current.name)
            put("server_addr", current.addr)
            put("master_key", current.key)
            put("sni", current.sni)
            put("enable_chunker", true)
        }.toString()

        val intent = Intent(this, ShadowStarVpnService::class.java).apply {
            action = ShadowStarVpnService.ACTION_CONNECT
            putExtra(ShadowStarVpnService.EXTRA_CONFIG_JSON, json)
        }
        startForegroundService(intent)
    }

    private fun deleteServer(idx: Int) {
        if (idx in serversList.indices) {
            val updated = serversList.toMutableList().apply { removeAt(idx) }
            serversList = updated
            if (activeServerIndex >= serversList.size && serversList.isNotEmpty()) {
                activeServerIndex = serversList.size - 1
            }
            saveServers()
            Toast.makeText(this, "Узел удален", Toast.LENGTH_SHORT).show()
        }
    }

    private fun addServer(item: ServerItem) {
        serversList = serversList + item
        activeServerIndex = serversList.size - 1
        saveServers()
        Toast.makeText(this, "Узел добавлен", Toast.LENGTH_SHORT).show()
    }

    private fun importFromUri(uri: String) {
        try {
            var raw = uri.trim()
            if (raw.startsWith("shadowstar://", ignoreCase = true)) {
                raw = raw.substring("shadowstar://".length)
            }
            var nodeName = ""
            val hashIdx = raw.indexOf("#")
            if (hashIdx != -1) {
                nodeName = try { java.net.URLDecoder.decode(raw.substring(hashIdx + 1), "UTF-8") } catch (e: Exception) { raw.substring(hashIdx + 1) }
                raw = raw.substring(0, hashIdx)
            }

            var parsed = false
            // 1. Try base64 JSON
            try {
                val cleanB64 = raw.replace("-", "+").replace("_", "/")
                val bytes = android.util.Base64.decode(cleanB64, android.util.Base64.DEFAULT)
                val jsonStr = String(bytes, Charsets.UTF_8)
                val obj = JSONObject(jsonStr)
                val addr = obj.optString("server_addr", obj.optString("addr", ""))
                val key = obj.optString("master_key", obj.optString("key", ""))
                val sni = obj.optString("sni", "www.microsoft.com")
                val name = if (nodeName.isNotEmpty()) nodeName else obj.optString("name", "ShadowStar Node")
                if (addr.isNotEmpty() && key.isNotEmpty()) {
                    val item = ServerItem("node-${System.currentTimeMillis()}", name, addr, key, sni, 45)
                    addServer(item)
                    parsed = true
                }
            } catch (e: Exception) {
                // not base64 json
            }

            // 2. Try key@host:port or host:port:key
            if (!parsed) {
                val parts = raw.split("@", "#")
                if (parts.size >= 2) {
                    val key = parts[0].trim()
                    val hostPart = parts[1].trim()
                    val name = if (nodeName.isNotEmpty()) nodeName else hostPart
                    val item = ServerItem("node-${System.currentTimeMillis()}", name, hostPart, key, "www.microsoft.com", 45)
                    addServer(item)
                    parsed = true
                } else if (raw.contains(":")) {
                    val colParts = raw.split(":")
                    if (colParts.size >= 3) {
                        val hostPart = "${colParts[0].trim()}:${colParts[1].trim()}"
                        val key = colParts[2].trim()
                        val sni = if (colParts.size >= 4) colParts[3].trim() else "www.microsoft.com"
                        val name = if (nodeName.isNotEmpty()) nodeName else hostPart
                        val item = ServerItem("node-${System.currentTimeMillis()}", name, hostPart, key, sni, 45)
                        addServer(item)
                        parsed = true
                    }
                }
            }

            if (!parsed) {
                Toast.makeText(this, "Не удалось распознать формат узла", Toast.LENGTH_SHORT).show()
            }
        } catch (e: Exception) {
            Toast.makeText(this, "Ошибка импорта ссылки: ${e.message}", Toast.LENGTH_SHORT).show()
        }
    }

    private fun saveServers() {
        val prefs = getSharedPreferences("shadowstar_prefs", MODE_PRIVATE)
        val arr = JSONArray()
        serversList.forEach { s ->
            arr.put(JSONObject().apply {
                put("id", s.id)
                put("name", s.name)
                put("addr", s.addr)
                put("key", s.key)
                put("sni", s.sni)
            })
        }
        prefs.edit().putString("servers", arr.toString()).putInt("active_idx", activeServerIndex).apply()
    }

    private fun loadServers() {
        val prefs = getSharedPreferences("shadowstar_prefs", MODE_PRIVATE)
        val raw = prefs.getString("servers", null)
        if (raw != null) {
            val arr = JSONArray(raw)
            val list = mutableListOf<ServerItem>()
            for (i in 0 until arr.length()) {
                val o = arr.getJSONObject(i)
                list.add(ServerItem(
                    o.getString("id"),
                    o.getString("name"),
                    o.getString("addr"),
                    o.getString("key"),
                    o.optString("sni", "www.microsoft.com"),
                    45
                ))
            }
            serversList = list
            activeServerIndex = prefs.getInt("active_idx", 0).coerceIn(0, (list.size - 1).coerceAtLeast(0))
        } else {
            // Default demo node
            serversList = listOf(
                ServerItem("node-demo-nl", "Demo Node (NL)", "demo-node.shadowstar.org:4433", "shadowstar-demo-key-v1", "www.microsoft.com", 35)
            )
        }
    }

    override fun onDestroy() {
        unregisterReceiver(vpnStateReceiver)
        super.onDestroy()
    }
}

data class ServerItem(
    val id: String,
    val name: String,
    val addr: String,
    val key: String,
    val sni: String,
    val ping: Int = 0
)

@Composable
fun MainScreen(
    isConnected: Boolean,
    speedDown: String,
    speedUp: String,
    servers: List<ServerItem>,
    selectedIndex: Int,
    onToggleConnect: () -> Unit,
    onSelectServer: (Int) -> Unit,
    onDeleteServer: (Int) -> Unit,
    onAddServer: (ServerItem) -> Unit
) {
    var showAddDialog by remember { mutableStateOf(false) }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .background(Color(0xFF090D16))
            .padding(16.dp),
        horizontalAlignment = Alignment.CenterHorizontally
    ) {
        // Top Brand Header
        Row(
            modifier = Modifier.fillMaxWidth().padding(vertical = 12.dp),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically
        ) {
            Column {
                Text("SHADOWSTAR", color = Color(0xFF00E5FF), fontSize = 20.sp, fontWeight = FontWeight.Bold, letterSpacing = 2.sp)
                Text("Stealth Reality VPN", color = Color(0xFF64748B), fontSize = 12.sp)
            }
            IconButton(onClick = { showAddDialog = true }) {
                Text("➕", fontSize = 20.sp)
            }
        }

        Spacer(modifier = Modifier.height(24.dp))

        // Center Big Power Button
        Box(
            modifier = Modifier
                .size(170.dp)
                .clip(CircleShape)
                .background(if (isConnected) Color(0xFF10B981) else Color(0xFF162032))
                .clickable { onToggleConnect() },
            contentAlignment = Alignment.Center
        ) {
            Column(horizontalAlignment = Alignment.CenterHorizontally) {
                Text("⏻", fontSize = 48.sp, color = if (isConnected) Color.White else Color(0xFF00E5FF))
                Spacer(modifier = Modifier.height(4.dp))
                Text(
                    text = if (isConnected) "ПОДКЛЮЧЕНО" else "ВКЛЮЧИТЬ",
                    color = Color.White,
                    fontSize = 14.sp,
                    fontWeight = FontWeight.Bold
                )
            }
        }

        Spacer(modifier = Modifier.height(24.dp))

        // Telemetry Card
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .clip(RoundedCornerShape(12.dp))
                .background(Color(0xFF101726))
                .padding(16.dp),
            horizontalArrangement = Arrangement.SpaceAround
        ) {
            Column(horizontalAlignment = Alignment.CenterHorizontally) {
                Text("СКАЧИВАНИЕ", color = Color(0xFF64748B), fontSize = 11.sp)
                Text("↓ $speedDown", color = Color(0xFF00E5FF), fontSize = 16.sp, fontWeight = FontWeight.Bold)
            }
            Column(horizontalAlignment = Alignment.CenterHorizontally) {
                Text("ОТДАЧА", color = Color(0xFF64748B), fontSize = 11.sp)
                Text("↑ $speedUp", color = Color(0xFF10B981), fontSize = 16.sp, fontWeight = FontWeight.Bold)
            }
        }

        Spacer(modifier = Modifier.height(24.dp))

        // Servers List Header
        Row(
            modifier = Modifier.fillMaxWidth().padding(horizontal = 4.dp),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically
        ) {
            Text("СПИСОК УЗЛОВ (${servers.size})", color = Color(0xFFCBD5E1), fontSize = 13.sp, fontWeight = FontWeight.SemiBold)
        }

        Spacer(modifier = Modifier.height(8.dp))

        if (servers.isEmpty()) {
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .height(140.dp)
                    .clip(RoundedCornerShape(12.dp))
                    .background(Color(0xFF101726)),
                contentAlignment = Alignment.Center
            ) {
                Text("Список серверов пуст.\nНажмите ➕ чтобы добавить узел.", color = Color(0xFF94A3B8), textAlign = androidx.compose.ui.text.style.TextAlign.Center)
            }
        } else {
            LazyColumn(modifier = Modifier.fillMaxWidth().weight(1f)) {
                itemsIndexed(servers) { idx, s ->
                    val isSelected = (idx == selectedIndex)
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(vertical = 4.dp)
                            .clip(RoundedCornerShape(10.dp))
                            .background(if (isSelected) Color(0xFF142436) else Color(0xFF101726))
                            .clickable { onSelectServer(idx) }
                            .padding(12.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.SpaceBetween
                    ) {
                        Column(modifier = Modifier.weight(1f)) {
                            Text(s.name, color = Color.White, fontWeight = FontWeight.SemiBold, fontSize = 15.sp)
                            Text(s.addr, color = Color(0xFF94A3B8), fontFamily = FontFamily.Monospace, fontSize = 12.sp)
                        }
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Text("${s.ping} ms", color = Color(0xFF10B981), fontSize = 13.sp, fontWeight = FontWeight.Bold)
                            Spacer(modifier = Modifier.width(12.dp))
                            IconButton(onClick = { onDeleteServer(idx) }) {
                                Text("✕", color = Color(0xFFEF4444), fontSize = 16.sp)
                            }
                        }
                    }
                }
            }
        }
    }

    if (showAddDialog) {
        var newName by remember { mutableStateOf("") }
        var newAddr by remember { mutableStateOf("") }
        var newKey by remember { mutableStateOf("") }

        AlertDialog(
            onDismissRequest = { showAddDialog = false },
            title = { Text("Новый узел ShadowStar") },
            text = {
                Column {
                    OutlinedTextField(value = newName, onValueChange = { newName = it }, label = { Text("Название") })
                    Spacer(modifier = Modifier.height(8.dp))
                    OutlinedTextField(value = newAddr, onValueChange = { newAddr = it }, label = { Text("Адрес (IP:Port)") })
                    Spacer(modifier = Modifier.height(8.dp))
                    OutlinedTextField(value = newKey, onValueChange = { newKey = it }, label = { Text("Мастер-ключ") })
                }
            },
            confirmButton = {
                Button(onClick = {
                    if (newAddr.isNotBlank() && newKey.isNotBlank()) {
                        val name = if (newName.isBlank()) newAddr else newName
                        onAddServer(ServerItem("node-${System.currentTimeMillis()}", name, newAddr, newKey, "www.microsoft.com", 0))
                        showAddDialog = false
                    }
                }) {
                    Text("Сохранить")
                }
            },
            dismissButton = {
                TextButton(onClick = { showAddDialog = false }) { Text("Отмена") }
            }
        )
    }
}

@Composable
fun ShadowStarMobileTheme(content: @Composable () -> Unit) {
    MaterialTheme(
        colorScheme = darkColorScheme(
            background = Color(0xFF090D16),
            surface = Color(0xFF101726),
            primary = Color(0xFF00E5FF)
        ),
        content = content
    )
}
