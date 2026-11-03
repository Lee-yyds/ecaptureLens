package dev.ecapturelens.ecapturelens

import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodChannel
import android.content.Intent
import android.os.Build
import android.Manifest
import android.content.pm.PackageManager

class MainActivity : FlutterActivity() {
    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)
        MethodChannel(flutterEngine.dartExecutor.binaryMessenger, "ecapturelens/core_service")
            .setMethodCallHandler { call, result ->
                val service = Intent(this, CoreService::class.java)
                try {
                    when (call.method) {
                        "start" -> {
                            if (Build.VERSION.SDK_INT >= 33 && checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
                                requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), 42)
                            }
                            if (Build.VERSION.SDK_INT >= 26) startForegroundService(service) else startService(service)
                            result.success(null)
                        }
                        "stop" -> { stopService(service); result.success(null) }
                        else -> result.notImplemented()
                    }
                } catch (error: RuntimeException) {
                    result.error("core_service", error.message, null)
                }
            }
    }
}
