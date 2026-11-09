Set-ItemProperty -Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings' -Name ProxyEnable -Value 0 -Type DWord
Remove-ItemProperty -Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings' -Name ProxyServer -ErrorAction SilentlyContinue

$connPath = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings\Connections'
foreach ($valName in @('DefaultConnectionSettings', 'SavedLegacySettings')) {
    $item = Get-ItemProperty -Path $connPath -Name $valName -ErrorAction SilentlyContinue
    if ($item) {
        $bytes = $item.$valName
        if ($bytes -and $bytes.Length -gt 8) {
            $bytes[8] = [byte]1
            Set-ItemProperty -Path $connPath -Name $valName -Value $bytes -Type Binary
        }
    }
}

$def = @"
using System;
using System.Runtime.InteropServices;
public class WinINet {
    [DllImport("wininet.dll", SetLastError = true)]
    public static extern bool InternetSetOption(IntPtr hInternet, int dwOption, IntPtr lpBuffer, int dwBufferLength);
}
"@
Add-Type -TypeDefinition $def -ErrorAction SilentlyContinue

[WinINet]::InternetSetOption([IntPtr]::Zero, 39, [IntPtr]::Zero, 0)
[WinINet]::InternetSetOption([IntPtr]::Zero, 37, [IntPtr]::Zero, 0)

Write-Host "Proxy successfully disabled and cleared."
