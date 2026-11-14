package com.shadowstar.vpn;

import android.content.Intent;
import android.net.VpnService;
import android.os.Build;
import android.service.quicksettings.Tile;
import android.service.quicksettings.TileService;

public class ShadowStarTileService extends TileService {

    public static volatile boolean isVpnActive = false;

    @Override
    public void onStartListening() {
        super.onStartListening();
        updateTileState();
    }

    @Override
    public void onClick() {
        super.onClick();
        Tile tile = getQsTile();
        if (tile == null) return;

        if (isVpnActive) {
            // Stop VPN
            Intent stopIntent = new Intent(this, ShadowStarVpnService.class);
            stopIntent.setAction("STOP");
            startService(stopIntent);
            isVpnActive = false;
        } else {
            // Check if VPN permission prepared
            Intent prepare = VpnService.prepare(this);
            if (prepare != null) {
                // Must launch activity to request user permission
                Intent appIntent = new Intent(this, MainActivity.class);
                appIntent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
                startActivityAndCollapse(appIntent);
                return;
            }

            // Start VPN
            Intent startIntent = new Intent(this, ShadowStarVpnService.class);
            startIntent.setAction("START");
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                startForegroundService(startIntent);
            } else {
                startService(startIntent);
            }
            isVpnActive = true;
        }
        updateTileState();
    }

    private void updateTileState() {
        Tile tile = getQsTile();
        if (tile == null) return;

        if (isVpnActive) {
            tile.setState(Tile.STATE_ACTIVE);
            tile.setLabel("ShadowStar: ВКЛ");
        } else {
            tile.setState(Tile.STATE_INACTIVE);
            tile.setLabel("ShadowStar");
        }
        tile.updateTile();
    }
}
